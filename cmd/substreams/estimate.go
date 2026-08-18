package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/internal/formatx"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var (
	// Styling for output - colors only enabled if terminal is TTY
	isTTY         = isatty.IsTerminal(os.Stdout.Fd())
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle    = lipgloss.NewStyle().Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	noteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	separatorChar = "─"
)

func init() {
	// Disable colors if not a TTY
	if !isTTY {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	// Add flags similar to sink noop but with estimate-specific flags
	sink.AddFlagsToSet(estimateCmd.Flags(),
		sink.FlagExcludeDefault(
			sink.FlagUndoBufferSize,
			sink.FlagDevelopmentMode,
			sink.FlagNoopMode,
			sink.FlagProtoPath,
			sink.FlagProtoDescriptorSet,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagFinalBlocksOnly,
		))

	estimateCmd.Flags().Float64("sample-percentage", 1, "Percentage of the requested range the server samples to measure the output size (server-side estimation only)")
	estimateCmd.Flags().Bool("local", false, "Sample from this client instead of asking the server: works against endpoints without server-side estimation, but only for single-stage modules and it pays for the egress of the sampled blocks")
	estimateCmd.Flags().Int("samples", 10, "Number of segments of 1000 blocks to estimate against (--local only)")
	estimateCmd.Flags().Int("parallel-requests", 1, "Number of parallel requests to perform (--local only)")
	estimateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt and proceed with estimation")

	rootCmd.AddCommand(estimateCmd)
}

var estimateCmd = &cobra.Command{
	Use:   "estimate [<manifest> [<module_name>]]",
	Short: "Estimate Substreams usage costs by sampling segments across the chain",

	Long: cli.Dedent(`
		Estimates the cost (processed blocks and egress bytes) of running a Substreams module
		over a block range, by running a small sample of that range and extrapolating.

		By default the endpoint does the work: it samples --sample-percentage of the range on its
		own workers, measures the size of the output it produced and reports back. The module data
		itself never leaves the server, so the estimation costs processed blocks but no egress. The
		server also knows what its cache already holds, so it reports how many blocks are actually
		left to process, and it handles modules with stores (whose segments cannot be run out of
		order: with stores, the sample is spread only over the part of the range the cache covers,
		or falls back to one contiguous run, which the report says).

		With --local the sampling is done from here instead, by streaming the sampled segments: use
		it against an endpoint that does not support server-side estimation. That mode only works
		with single-stage modules (mappers only, no stores) and it pays for the egress of every
		sampled block.

		Limitations and Warnings:
		  - Results are approximations based on sampling and can differ significantly from actual usage
		  - A non-representative sample (e.g., sampling only low-activity or high-activity periods)
		    can severely skew results
		  - With --local, processed block estimation may be inaccurate for modules with block filters
		    or indexes, as server-side caching and filtering are not fully accounted for

		This is a best-effort approximation tool. Actual costs may vary, sometimes greatly, from
		these estimates. Ensure that you use a representative sample range to improve accuracy of
		the estimates.
	`),
	RunE: estimateE,
	Args: cobra.RangeArgs(0, 2),
}

func estimateE(cmd *cobra.Command, args []string) error {
	logging.SetLevelFor(".*", zap.DPanicLevel, false)

	ctx := cmd.Context()
	cmd.SilenceUsage = true

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

	// Load auth environment file if it exists
	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	// Parse flags to get sinker config
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink/estimate", zlog, tracer)
	if err != nil {
		return err
	}

	// Force production mode for estimation and infinite retries
	sinkerConfig.Mode = sink.SubstreamsModeProduction
	sinkerConfig.MaxRetries = -1

	if !sflags.MustGetBool(cmd, "local") {
		return runServerEstimate(ctx, cmd, sinkerConfig, sflags.MustGetFloat64(cmd, "sample-percentage"))
	}

	// Get estimate-specific flags
	numSegments := sflags.MustGetInt(cmd, "samples")
	parallelRequests := sflags.MustGetInt(cmd, "parallel-requests")

	zlog.Info("starting substreams estimation",
		zap.String("manifest", manifestPath),
		zap.String("output_module", sinkerConfig.OutputModule.Name),
		zap.Int("chain_segments", numSegments),
		zap.Int("parallel_requests", parallelRequests),
	)

	graph, err := exec.NewOutputModuleGraph(outputModule, true, sinkerConfig.Pkg.Modules, 0)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	if err := validateSingleStageModule(graph); err != nil {
		return err
	}

	stopBlock := sinkerConfig.StopBlock
	if stopBlock == 0 {
		stopBlock, err = fetchHeadBlock(ctx, sinkerConfig)
		if err != nil {
			return fmt.Errorf("fetching head block: %w", err)
		}

		zlog.Info("fetched head block", zap.Uint64("head_block", stopBlock))
		sinkerConfig.StopBlock = stopBlock
	}

	startBlock := uint64(sinkerConfig.StartBlock)
	if startBlock == 0 {
		startBlock = graph.OutputModule().InitialBlock
	} else {
		// Force the module to resolve to our start block, to avoid problem where use "forgets"
		// that it set an higher start block than the module initial block.
		for _, module := range sinkerConfig.Pkg.Modules.Modules {
			module.InitialBlock = startBlock
		}
	}

	segments := calculateChainSegments(startBlock, stopBlock, numSegments)

	zlog.Info("calculated chain segments",
		zap.Int("num_segments", len(segments)),
		zap.Uint64("start_block", startBlock),
		zap.Uint64("end_block", stopBlock),
	)

	// Calculate total sampling coverage
	var totalSampledBlocks uint64
	for _, seg := range segments {
		totalSampledBlocks += seg.SampledEndBlock - seg.SampledStartBlock + 1
	}
	totalRange := stopBlock - startBlock
	samplingPercentage := float64(totalSampledBlocks) / float64(totalRange) * 100.0

	report := NewReportBuilder()

	report.Line(titleStyle.Render("Substreams Cost Estimation"))
	report.Line(dimStyle.Render(sectionSeparator))
	report.Line("%s %s", labelStyle.Render("Package:"), valueStyle.Render(sinkerConfig.Pkg.PackageMeta[0].Name))
	report.Line("%s  %s", labelStyle.Render("Module:"), valueStyle.Render(sinkerConfig.OutputModule.Name))
	report.Line("%s  %s", labelStyle.Render("Range:"), valueStyle.Render(fmt.Sprintf("%s - %s (%s blocks)",
		formatx.Integer(startBlock),
		formatx.Integer(stopBlock),
		formatx.Integer(totalRange))))
	report.Line("%s %s", labelStyle.Render("Samples:"), valueStyle.Render(fmt.Sprintf("%d segments (%s blocks, %.2f%% of range)",
		len(segments),
		formatx.Integer(totalSampledBlocks),
		samplingPercentage)))
	report.Line("%s %s", labelStyle.Render("Workers:"), valueStyle.Render(fmt.Sprintf("%d parallel", parallelRequests)))
	report.Line(dimStyle.Render(sectionSeparator))
	report.Line("")

	// Ask for confirmation before proceeding
	skipConfirmation := sflags.MustGetBool(cmd, "yes")
	if !skipConfirmation {
		report.Line(labelStyle.Render("This estimation will:"))
		report.Line("  • Sample %s blocks (%.2f%% of the requested range)",
			formatx.Integer(totalSampledBlocks),
			samplingPercentage)
		report.Line("  • Consume %s processed blocks from your plan's quota",
			formatx.Integer(totalSampledBlocks))
		report.Line("  • Generate egress bytes that count toward your plan's limits")
		report.Line("")
		report.Line(labelStyle.Render("Estimation accuracy depends on:"))
		report.Line("  • Representative sampling across the block range")
		report.Line("  • Network conditions and client processing speed")
		report.Line("  • Server-side caching and filtering behavior")
		report.Line("")
		report.Line(noteStyle.Render("Results are approximations and may differ significantly from actual usage,"))
		report.Line(noteStyle.Render("especially if the sample is not representative (e.g., sampling only low-activity"))
		report.Line(noteStyle.Render("or high-activity periods)."))

		if !isTTY {
			report.Line(errorStyle.Render("Error: Terminal is not interactive. Use the -y or --yes flag to proceed without confirmation."))
			return fmt.Errorf("non-interactive terminal requires --yes flag")
		}

		report.Line("")
		confirmed, wasAnswered := cli.PromptConfirm("Do you want to proceed with this estimation?")
		if !wasAnswered || !confirmed {
			report.Line("")
			report.Line("Estimation cancelled.")
			return nil
		}

		report.Line("")
		report.Line(dimStyle.Render(sectionSeparator))
		report.Line("")
	}

	report.Line(headerStyle.Render(fmt.Sprintf("Estimation starting (%d segments, parallel worker %d)",
		len(segments),
		parallelRequests,
	)))

	results, err := runSegmentsParallel(ctx, segments, parallelRequests, sinkerConfig, graph)
	if err != nil {
		return fmt.Errorf("running segments: %w", err)
	}
	report.Line("")

	generateReport(report, results, startBlock, stopBlock)

	// Print usage report (actual consumption during estimation across all segments)
	printUsageReport()

	return nil
}

// ChainSegment represents a segment to test and the chain range it represents
type ChainSegment struct {
	// The 1000-block range to actually test
	SampledStartBlock uint64
	SampledEndBlock   uint64

	// The full chain range this segment represents
	ChainStartBlock uint64
	ChainEndBlock   uint64
}

// SegmentResult contains the metrics collected from running a segment
type SegmentResult struct {
	Segment             ChainSegment
	ProcessedBlockCount uint64
	ReceivedBlockCount  uint64
	EgressBytes         uint64
	Duration            time.Duration
	Error               error
}

// SampledRangeString returns a formatted string for the sampled block range
func (r *SegmentResult) SampledRangeString() string {
	return fmt.Sprintf("%s - %s",
		formatx.Integer(r.Segment.SampledStartBlock),
		formatx.Integer(r.Segment.SampledEndBlock))
}

// ChainRangeString returns a formatted string for the chain block range
func (r *SegmentResult) ChainRangeString() string {
	return fmt.Sprintf("%s - %s",
		formatx.Integer(r.Segment.ChainStartBlock),
		formatx.Integer(r.Segment.ChainEndBlock))
}

// ChainRange returns the number of blocks in the chain segment
func (r *SegmentResult) ChainRange() uint64 {
	return r.Segment.ChainEndBlock - r.Segment.ChainStartBlock
}

// SampledRange returns the number of blocks in the sampled segment
func (r *SegmentResult) SampledRange() uint64 {
	return r.Segment.SampledEndBlock - r.Segment.SampledStartBlock + 1
}

// substreamsBlockCountInOneJob is the fixed to the number of block Substreams runs in a single job
const substreamsBlockCountInOneJob = float64(1000)

// Substreams worker ramp-up constants
const (
	workerToStartPerTimeframe = 5
	workerTimeframe           = 1 * time.Second
)

// ForecastProcessingTime estimates how long it would take to process the full chain segment
// with the given number of parallel workers. Returns nil if there was an error.
// Includes worker ramp-up time to reach the target worker count.
func (r *SegmentResult) ForecastProcessingTime(workerCount int) *time.Duration {
	if r.Error != nil {
		return nil
	}

	chainBlockCount := r.ChainRange()
	sampledBlockCount := r.SampledRange()

	zlog.Debug("forecasting segment",
		zap.Int("worker_count", workerCount),
		zap.Uint64("chain_block_count", chainBlockCount),
		zap.Uint64("sampled_block_count", sampledBlockCount),
		zap.Float64("substreams_block_count_in_one_job", substreamsBlockCountInOneJob),
	)

	substreamsJobCount := int(math.Ceil(float64(chainBlockCount) / substreamsBlockCountInOneJob))
	timePerSubstreamsJob := r.Duration / time.Duration(sampledBlockCount/uint64(substreamsBlockCountInOneJob))

	zlog.Debug("forecasting chain range job count and time per job",
		zap.Int("worker_count", workerCount),
		zap.Int("substreams_job_count", substreamsJobCount),
		zap.Duration("time_per_substreams_job", timePerSubstreamsJob),
	)

	// We can run `workerCount` substreams jobs in parallel, a bucket represent a set of `workerCount`
	// jobs, how many sets do we need to run all jobs is the parallelBucketCount.
	//
	// Let's use an example 50 jobs and 15 workers, we can run 15 jobs in parallel, so we need to run
	// 4 sets of jobs (buckets) to complete all 50 jobs (15 + 15 + 15 + 5). Assuming that each job
	// takes the same time, let's use 1 minute, it each 1 minute, we complete 15 jobs, so in 4 minutes
	// we complete all 50 jobs, hence the `time.Duration(parallelBucketCount) * timePerSubstreamsJob`.
	parallelBucketCount := int(math.Ceil(float64(substreamsJobCount) / float64(workerCount)))
	chainRangeDuration := time.Duration(parallelBucketCount) * timePerSubstreamsJob

	zlog.Debug("forecasted chain range processing time",
		zap.Int("worker_count", workerCount),
		zap.Int("parallel_bucket_count", parallelBucketCount),
		zap.Duration("chain_range_duration", chainRangeDuration),
	)

	return &chainRangeDuration
}

// ForecastProcessedBlocks returns the estimated processed blocks for the full chain segment
func (r *SegmentResult) ForecastProcessedBlocks() uint64 {
	chainRange := r.ChainRange()
	sampledRange := r.SampledRange()
	scaleFactor := float64(chainRange) / float64(sampledRange)
	return uint64(float64(r.ProcessedBlockCount) * scaleFactor)
}

// ForecastEgressBytes returns the estimated egress bytes for the full chain segment
func (r *SegmentResult) ForecastEgressBytes() uint64 {
	chainRange := r.ChainRange()
	sampledRange := r.SampledRange()
	scaleFactor := float64(chainRange) / float64(sampledRange)

	zlog.Debug("forecasting egress bytes",
		zap.Uint64("chain_range", chainRange),
		zap.Uint64("sampled_range", sampledRange),
		zap.Float64("scale_factor", scaleFactor),
	)

	return uint64(float64(r.EgressBytes) * scaleFactor)
}

type SegmentResults []*SegmentResult

// ForecastProcessingTime estimates how long it would take to reprocess the entire chain
// with different numbers of parallel workers, fixed to 5, 15, 75 and 200 workers which are
// the keys of the returned map, the value being the total forecasted duration with this
// many workers.
func (results SegmentResults) ForecastProcessingTime() (out map[int]time.Duration) {
	out = map[int]time.Duration{
		5:   0,
		15:  0,
		75:  0,
		200: 0,
	}

	for _, result := range results {
		if result.Error != nil {
			continue
		}

		for workerCount := range out {
			if forecastTime := result.ForecastProcessingTime(workerCount); forecastTime != nil {
				out[workerCount] += *forecastTime
			}
		}
	}

	// Add worker ramp-up time once per worker count
	// Workers start at a rate of workerToStartPerTimeframe per workerTimeframe
	// Example: 200 workers at 5 workers/second = 40 seconds ramp-up time
	for workerCount := range out {
		rampUpTime := time.Duration(float64(workerCount)/workerToStartPerTimeframe) * workerTimeframe
		out[workerCount] += rampUpTime
	}

	if zlog.Core().Enabled(zap.DebugLevel) {
		perWorkerCountDurations := make([]string, 0, len(out))
		for workerCount, duration := range out {
			perWorkerCountDurations = append(perWorkerCountDurations, fmt.Sprintf("%d workers: %s", workerCount, duration))
		}

		zlog.Debug("reprocessing time estimate",
			zap.Reflect("report", perWorkerCountDurations),
		)
	}

	return out
}

// ForecastEgressBytes returns the estimated egress bytes for the all the results, which is
// the sum of all individual segment forecasts and represent the full estimation range as specified
// in the command.
func (results SegmentResults) ForecastEgressBytes() uint64 {
	var totalEgress uint64
	for _, result := range results {
		if result.Error != nil {
			continue
		}
		totalEgress += result.ForecastEgressBytes()
	}
	return totalEgress
}

func (results SegmentResults) ForecastProcessedBlocks() uint64 {
	var totalProcessedBlocks uint64
	for _, result := range results {
		if result.Error != nil {
			continue
		}
		totalProcessedBlocks += result.ForecastProcessedBlocks()
	}
	return totalProcessedBlocks
}

// validateSingleStageModule ensures the module graph has only one stage with only mappers
func validateSingleStageModule(graph *exec.Graph) error {
	stages := graph.StagedUsedModules()

	if len(stages) != 1 {
		return fmt.Errorf("estimation only works with single-stage modules (modules with only mappers, no stores), found %d stages", len(stages))
	}

	// Check that all modules in the stage are mappers (not stores)
	for _, layer := range stages[0] {
		for _, module := range layer {
			if module.GetKindStore() != nil {
				return fmt.Errorf("estimation only works with mapper modules, found store module: %s", module.Name)
			}
		}
	}

	return nil
}

// hasFilteredModules checks if any modules in the dependency chain from the output module have block filters.
// Returns true if any module has a block filter defined, which could affect estimation accuracy.
func hasFilteredModules(graph *exec.Graph) bool {
	// Check all used modules (which includes all dependencies of the output module)
	for _, module := range graph.UsedModules() {
		if module.BlockFilter != nil {
			return true
		}
	}
	return false
}

// fetchHeadBlock fetches the current head block by requesting the common package at head
func fetchHeadBlock(ctx context.Context, clientConfig *sink.SinkerConfig) (uint64, error) {
	fetchHeaderConfig, err := clientConfig.CloneWithNewModule(sink.IgnoreOutputModuleType, "common@v0.1.0", "map_clocks")
	if err != nil {
		return 0, fmt.Errorf("cloning client config for head block fetch: %w", err)
	}

	fetchHeaderConfig.StartBlock = -1
	fetchHeaderConfig.StopBlock = 0

	sinker, err := sink.NewFromConfig(fetchHeaderConfig)
	if err != nil {
		return 0, fmt.Errorf("creating sinker: %w", err)
	}

	headBlock := uint64(0)

	sinker.Run(ctx, nil, sink.NewSinkerHandlers(
		func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
			headBlock = data.Clock.Number
			sinker.Shutdown(nil)
			return nil
		},
		func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
			return nil
		},
	))
	if err := sinker.Err(); err != nil && err != context.Canceled {
		return 0, fmt.Errorf("sinker run error: %w", err)
	}

	return headBlock, nil
}

// calculateChainSegments divides the chain range into evenly distributed segments on 1000-block boundaries
func calculateChainSegments(startBlock, endBlock uint64, numSegments int) []ChainSegment {
	if numSegments <= 0 {
		numSegments = 1
	}

	totalRange := endBlock - startBlock
	if totalRange == 0 {
		return nil
	}

	// Calculate the chain range each segment represents
	chainRangePerSegment := totalRange / uint64(numSegments)

	segments := make([]ChainSegment, 0, numSegments)

	for i := 0; i < numSegments; i++ {
		// Calculate the chain range this segment represents
		chainStart := startBlock + uint64(i)*chainRangePerSegment
		var chainEnd uint64
		if i == numSegments-1 {
			// Last segment goes to the end
			chainEnd = endBlock
		} else {
			chainEnd = chainStart + chainRangePerSegment
		}

		// Round chainStart up to nearest 1000-block boundary
		testStart := ((chainStart + 999) / 1000) * 1000
		testEnd := testStart + 999 // Test exactly 1000 blocks

		// Make sure testEnd doesn't exceed the chain end
		if testEnd > chainEnd {
			testEnd = chainEnd
		}

		// Skip if the range is invalid (testStart must be strictly less than testEnd)
		if testStart >= testEnd {
			continue
		}

		segments = append(segments, ChainSegment{
			SampledStartBlock: testStart,
			SampledEndBlock:   testEnd,
			ChainStartBlock:   chainStart,
			ChainEndBlock:     chainEnd,
		})
	}

	return segments
}

// runSegmentsParallel runs all segments in parallel with a worker pool
func runSegmentsParallel(ctx context.Context, segments []ChainSegment, maxParallel int, baseConfig *sink.SinkerConfig, graph *exec.Graph) ([]*SegmentResult, error) {
	results := make([]*SegmentResult, len(segments))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	errChan := make(chan error, len(segments))

	// Progress tracking
	progressChan := make(chan *SegmentResult, len(segments))
	doneChan := make(chan struct{})

	// Start progress reporter
	go displayProgress(progressChan, doneChan, len(segments))

	// Launch workers for each segment
	for i, segment := range segments {
		sem <- struct{}{} // Acquire semaphore (blocks if maxParallel reached)
		wg.Add(1)

		go func(idx int, seg ChainSegment) {
			defer func() {
				<-sem // Release semaphore
				wg.Done()
			}()

			result := runSegment(ctx, seg, baseConfig, graph)
			results[idx] = result

			// Send progress update
			progressChan <- result

			if result.Error != nil {
				errChan <- result.Error
			}
		}(i, segment)
	}

	// Wait for all workers to complete
	wg.Wait()

	close(progressChan)
	<-doneChan // Wait for progress reporter to finish
	close(errChan)

	var allError error
	for err := range errChan {
		allError = errors.Join(allError, err)
	}

	return results, allError
}

// runSegment runs a single segment and collects metrics
func runSegment(ctx context.Context, segment ChainSegment, baseConfig *sink.SinkerConfig, graph *exec.Graph) *SegmentResult {
	result := &SegmentResult{
		Segment: segment,
	}

	// Create a copy of the config for this segment
	config := *baseConfig
	config.StartBlock = int64(segment.SampledStartBlock)
	config.StopBlock = segment.SampledEndBlock + 1 // Stop block is exclusive
	config.Mode = sink.SubstreamsModeProduction

	sinker, err := sink.NewFromConfig(&config)
	if err != nil {
		result.Error = fmt.Errorf("creating sinker: %w", err)
		return result
	}

	// Track metrics
	var blockCount uint64
	var egressBytes uint64

	handlers := sink.NewSinkerFullHandlers(
		func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
			// FIXME: Let the server reports the egress bytes without transferring and without billing it of course,
			// this way estimate would cost only block count for the estimation.

			egressBytes += uint64(proto.Size(data))
			blockCount++
			return nil
		},
		func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
			return nil
		},
		func(ctx context.Context, req *pbsubstreamsrpcv3.Request, session *pbsubstreamsrpc.SessionInit) error {
			zlog.Info("started segment",
				zap.Reflect("session", session),
				zap.Uint64("sampled_start_block", segment.SampledStartBlock),
				zap.Uint64("sampled_end_block", segment.SampledEndBlock),
			)
			return nil
		},
		nil, // progress message
		nil, // initial snapshot data
		nil, // initial snapshot complete
		nil, // error handler
	)

	startTime := time.Now()

	sinker.Run(ctx, nil, handlers)

	select {
	case <-time.After(15 * time.Minute):
		result.Error = fmt.Errorf("segment processing timed out after 15 minutes")
		sinker.Shutdown(nil)
	case <-sinker.Terminated():
	default:
	}

	result.Duration = time.Since(startTime)

	// Counting properly the processed blocks is a bit complicated due to caching of the production mode.
	// If a segment is fully cached already, no blocks processed will be reported, but for an estimation,
	// a 0 number don't make sense.
	//
	// To be accurate, state need to be accounted for server side and reported. One thing client side
	// we could do is to find the all mappers that has a block source and is using an index and have a query.
	// We would run all those found modules in production mode and, how many unique block number in the range
	// across those modules would be the processed block count.
	//
	// As a simpler approximation, if there is any filter done in the module chain, we use the ReceivedBlockCount
	// as the processed block count, otherwise we use the full segment range of the sample which is usually 1000 blocks.

	if hasFilteredModules(graph) {
		result.ProcessedBlockCount = blockCount
	} else {
		result.ProcessedBlockCount = segment.SampledEndBlock - segment.SampledStartBlock + 1
	}
	result.ReceivedBlockCount = blockCount
	result.EgressBytes = egressBytes
	result.Error = sinker.Err()

	if result.Error == context.Canceled || result.Error == context.DeadlineExceeded {
		result.Error = nil // These are expected when we cancel
	}

	// It seems the Substreams server requires a bit of time to cleanup resources so that our active connections is not
	// reached rapidly. Would need to investigate further why this is happening.
	<-time.After(5 * time.Second)

	return result
}

// displayProgress shows progress of running segments
func displayProgress(progressChan <-chan *SegmentResult, doneChan chan struct{}, totalSegments int) {
	defer close(doneChan)

	completed := 0
	for result := range progressChan {
		completed++
		status := "✓"
		if result.Error != nil {
			status = "✗"
		}

		fmt.Printf("[%s] Segment %d/%d: Blocks %s - %s (%s, %s processed blocks, %s egress)\n",
			status,
			completed,
			totalSegments,
			formatx.Integer(result.Segment.SampledStartBlock),
			formatx.Integer(result.Segment.SampledEndBlock),
			formatx.Duration(result.Duration, formatx.WithZero("N/A")),
			formatx.Integer(result.ProcessedBlockCount),
			formatx.Bytes(result.EgressBytes),
		)
	}
}

var (
	sectionSeparator = strings.Repeat(separatorChar, 105)
)

// generateReport creates a human-readable report of the estimation results
func generateReport(report *ReportBuilder, results SegmentResults, startBlock, endBlock uint64) {
	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.LineStyled(headerStyle, "Partial Segments Forecast")
	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.Line("")

	totalRange := endBlock - startBlock

	// Calculate aggregated metrics
	var totalDuration time.Duration
	var totalSampledBlocks uint64
	successfulSegments := 0

	// Build table data
	var tableData [][]string
	for i, result := range results {
		if result.Error != nil {
			tableData = append(tableData, []string{
				fmt.Sprintf("%d", i+1),
				"ERROR",
				"",
				"",
				"",
				result.Error.Error(),
			})
			continue
		}

		successfulSegments++
		totalDuration += result.Duration
		totalSampledBlocks += result.SampledRange()

		tableData = append(tableData, []string{
			fmt.Sprintf("%d", i+1),
			result.SampledRangeString(),
			result.ChainRangeString(),
			fmt.Sprintf("%.2fs", result.Duration.Seconds()),
			formatx.Integer(result.ForecastProcessedBlocks()),
			formatx.Bytes(result.ForecastEgressBytes()),
		})
	}

	generateSegmentsTable(report, tableData)
	report.Line("")

	if successfulSegments == 0 {
		report.Line("")
		report.LineStyled(errorStyle, "⚠ No successful segments to generate forecast")
		return
	}

	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.LineStyled(headerStyle, "Full Range Forecast")
	report.LineStyled(dimStyle, "%s", sectionSeparator)
	report.Line("")

	samplingPercentage := float64(totalSampledBlocks) / float64(totalRange) * 100.0

	report.Line("%s %s",
		labelStyle.Render("Requested Range:"),
		valueStyle.Render(fmt.Sprintf("%s - %s (%s blocks)",
			formatx.Integer(startBlock),
			formatx.Integer(endBlock),
			formatx.Integer(totalRange))))
	report.Line("%s %s",
		labelStyle.Render("Blocks Sampled:"),
		valueStyle.Render(fmt.Sprintf("%s (%.2f%% of range)",
			formatx.Integer(totalSampledBlocks),
			samplingPercentage)))
	report.Line("%s %s",
		labelStyle.Render("Segments Tested:"),
		valueStyle.Render(fmt.Sprintf("%d successful / %d total", successfulSegments, len(results))))
	report.Line("%s %s",
		labelStyle.Render("Test Duration:"),
		valueStyle.Render(fmt.Sprintf("%.2fs", totalDuration.Seconds())))
	report.Line("")

	report.Line("%s %s",
		labelStyle.Render("Estimated Processed Blocks:"),
		successStyle.Render(formatx.Integer(results.ForecastProcessedBlocks())))
	report.Line("%s %s",
		labelStyle.Render("Estimated Egress Bytes:"),
		successStyle.Render(formatx.Bytes(results.ForecastEgressBytes())))
	report.Line("")

	// Calculate estimated reprocessing time
	reprocessingTimes := results.ForecastProcessingTime()
	report.LineStyled(labelStyle, "Estimated Reprocessing Time:")
	report.Line("  %s %s",
		dimStyle.Render("Free plan (5 workers):"),
		valueStyle.Render(formatx.Duration(reprocessingTimes[5], formatx.WithZero("N/A"))))
	report.Line("  %s %s",
		dimStyle.Render("Scaling plan (15 workers):"),
		valueStyle.Render(formatx.Duration(reprocessingTimes[15], formatx.WithZero("N/A"))))
	report.Line("  %s %s",
		dimStyle.Render("Pro plan (75 workers):"),
		valueStyle.Render(formatx.Duration(reprocessingTimes[75], formatx.WithZero("N/A"))))
	report.Line("  %s %s",
		dimStyle.Render("Enterprise plan (200 workers):"),
		valueStyle.Render(formatx.Duration(reprocessingTimes[200], formatx.WithZero("N/A"))))
	report.Line("")
	report.LineStyled(dimStyle, "%s", sectionSeparator)
}

func generateSegmentsTable(report *ReportBuilder, tableData [][]string) {
	renderReportTable(report, []string{"#", "Test Range", "Chain Range", "Duration", "Est. Blocks", "Est. Egress"}, tableData)
}

func renderReportTable(report *ReportBuilder, headers []string, tableData [][]string) {
	if len(tableData) == 0 {
		return
	}

	firstRow := tableData[0]
	if len(firstRow) != len(headers) {
		panic(fmt.Errorf("first row length %d does not match headers length %d (headers %v, first row %v)",
			len(firstRow),
			len(headers),
			headers,
			firstRow,
		))
	}

	// Borderless, separator-less layout matching the previous rendering.
	settings := tw.Settings{Separators: tw.SeparatorsNone, Lines: tw.LinesNone}
	var rnd tw.Renderer
	if isTTY {
		// Cyan bold headers via the colorized renderer.
		rnd = renderer.NewColorized(renderer.ColorizedConfig{
			Borders:  tw.BorderNone,
			Settings: settings,
			Header:   renderer.Tint{FG: renderer.Colors{color.FgCyan, color.Bold}},
		})
	} else {
		rnd = renderer.NewBlueprint(tw.Rendition{Borders: tw.BorderNone, Settings: settings})
	}

	// Uppercase headers ourselves and disable auto-formatting: v1's auto-format
	// splits on punctuation (e.g. "Est. Blocks" -> "EST . BLOCKS").
	upperHeaders := make([]string, len(headers))
	for i, h := range headers {
		upperHeaders[i] = strings.ToUpper(h)
	}

	table := tablewriter.NewTable(report,
		tablewriter.WithRenderer(rnd),
		tablewriter.WithHeaderAutoFormat(tw.Off),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoWrap(tw.WrapNone),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
		tablewriter.WithPadding(tw.Padding{Right: "  "}),
	)
	table.Header(upperHeaders)
	_ = table.Bulk(tableData)
	_ = table.Render()
}

var _ io.Writer = (*ReportBuilder)(nil)

type ReportBuilder struct {
	writer io.Writer
}

func NewReportBuilder() *ReportBuilder {
	return &ReportBuilder{
		writer: os.Stdout,
	}
}

// Write implements io.Writer.
func (r *ReportBuilder) Write(p []byte) (n int, err error) {
	return r.writer.Write(p)
}

func (r *ReportBuilder) Line(format string, a ...any) {
	fmt.Fprintf(r.writer, format+"\n", a...)
}

func (r *ReportBuilder) LineStyled(style lipgloss.Style, format string, a ...any) {
	fmt.Fprintf(r.writer, "%s", style.Render(fmt.Sprintf(format, a...))+"\n")
}

// printUsageReport prints the actual usage consumed during estimation by reading
// from the global metrics that were accumulated across all segment executions
func printUsageReport() {
	egressBytes := sink.ServerEgressBytes.Get()
	processedBlocks := sink.ProcessedBlocks.Get()
	receivedBlockData := sink.DataMessageCount.Get()

	var noDataReceived string
	if egressBytes == 0 && processedBlocks == 0 {
		noDataReceived = " (no data received)"
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "📊 Usage Report%s\n", noDataReceived)
	fmt.Fprintf(os.Stderr, " • Egress Bytes (uncompressed): %s\n", formatx.Bytes(uint64(egressBytes)))
	fmt.Fprintf(os.Stderr, " • Processed Blocks: %s blocks\n", formatx.Integer(uint64(processedBlocks)))
	fmt.Fprintf(os.Stderr, " • Received Blocks: %s blocks\n", formatx.Integer(uint64(receivedBlockData)))
}
