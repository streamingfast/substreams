package sink

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/client"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// IgnoreOutputModuleType can be used instead of the expected output module type
// when you want to validate this yourself, for example if you accept multiple
// output type(s).
const IgnoreOutputModuleType string = "@!##_IgnoreOutputModuleType_##!@"

// InferOutputModuleFromPackage can be used instead of the actual module's output name
// and has the effect that output module is extracted directly from the [pbsubstreams.Package]
// via the `SinkModule` field.
const InferOutputModuleFromPackage string = "@!##_InferOutputModuleFromSpkg_##!@"

type Sinker struct {
	*shutter.Shutter

	// Constructor (ordered)
	mode             SubstreamsMode
	NoopMode         bool
	pkg              *pbsubstreams.Package
	outputModule     *pbsubstreams.Module
	outputModuleHash string
	clientConfig     *client.SubstreamsClientConfig
	logger           *zap.Logger
	tracer           logging.Tracer

	// Options
	backOff    backoff.BackOff
	buffer     *blockDataBuffer
	startBlock int64
	stopBlock  uint64
	request    *pbsubstreamsrpc.Request

	infiniteRetry   bool
	finalBlocksOnly bool
	livenessChecker LivenessChecker
	extraHeaders    []string

	// State
	stats                   *Stats
	requestActiveStartBlock uint64
}

// New creates a new Sinker instance from the provided SinkerConfig.
// This function replaces the previous Options pattern with a more structured
// configuration approach. All configuration is now contained within the
// SinkerConfig struct, making it easier to manage and test.
func New(
	config *SinkerConfig,
	logger *zap.Logger,
	tracer logging.Tracer,
) *Sinker {
	s := &Sinker{
		Shutter:          shutter.New(),
		clientConfig:     config.ClientConfig,
		pkg:              config.Pkg,
		outputModule:     config.OutputModule,
		outputModuleHash: hex.EncodeToString(config.OutputModuleHash),
		mode:             config.Mode,
		NoopMode:         config.NoopMode,
		backOff:          config.BackOff,
		startBlock:       config.StartBlock,
		stopBlock:        config.StopBlock,
		infiniteRetry:    config.InfiniteRetry,
		finalBlocksOnly:  config.FinalBlocksOnly,
		livenessChecker:  config.LivenessChecker,
		extraHeaders:     config.ExtraHeaders,
		stats:            newStats(logger),
		logger:           logger,
		tracer:           tracer,
	}

	// Set up buffer unless final blocks only is configured
	if config.UndoBufferSize > 0 && !config.FinalBlocksOnly {
		s.buffer = newBlockDataBuffer(config.UndoBufferSize)
	}

	if s.finalBlocksOnly && s.buffer != nil {
		s.logger.Debug("discarding undo buffer since final blocks only requested")
		s.buffer = nil
	}

	s.logger.Info("sinker configured",
		zap.Stringer("mode", s.mode),
		zap.Int("module_count", len(s.pkg.Modules.Modules)),
		zap.String("output_module_name", s.OutputModuleName()),
		zap.String("output_module_type", s.outputModule.Output.Type),
		zap.String("output_module_hash", s.outputModuleHash),
		zap.Stringer("client_config", (*substramsClientStringer)(s.clientConfig)),
		zap.Stringer("buffer", s.buffer),
		zap.Int64("start_block", s.startBlock),
		zap.Uint64("stop_block", s.stopBlock),
		zap.Bool("infinite_retry", s.infiniteRetry),
		zap.Bool("final_blocks_only", s.finalBlocksOnly),
		zap.Bool("liveness_checker", s.livenessChecker != nil),
	)

	return s
}

type substramsClientStringer client.SubstreamsClientConfig

func (s *substramsClientStringer) String() string {
	config := (*client.SubstreamsClientConfig)(s)

	return fmt.Sprintf("%s (insecure: %t, plaintext: %t, JWT present: %t)", config.Endpoint(), config.Insecure(), config.PlainText(), config.AuthToken() != "")
}

func (s *Sinker) StartBlock() int64 {
	return s.startBlock
}

func (s *Sinker) StopBlock() uint64 {
	return s.stopBlock
}

func (s *Sinker) Package() *pbsubstreams.Package {
	return s.pkg
}

func (s *Sinker) BytesRepresentation() dynamic.BytesRepresentation {
	var network, endpoint string
	if s.pkg != nil {
		network = s.pkg.Network
	}
	if s.clientConfig != nil {
		endpoint = s.clientConfig.Endpoint()
	}
	return InferBytesRepresentation(network, endpoint)
}

func (s *Sinker) Request() *pbsubstreamsrpc.Request {
	return s.request
}

func (s *Sinker) OutputModule() *pbsubstreams.Module {
	return s.outputModule
}

// OutputModuleHash returns the module output hash, can be used by consumer
// to warn if the module changed between restart of the process.
func (s *Sinker) OutputModuleHash() string {
	return s.outputModuleHash
}

func (s *Sinker) OutputModuleName() string {
	return s.outputModule.Name
}

// OutputModuleTypePrefixed returns the prefixed output module's type so the type
// will always be prefixed with "proto:".
func (s *Sinker) OutputModuleTypePrefixed() (prefixed string) {
	_, prefixed = sanitizeModuleType(s.outputModule.Output.Type)
	return
}

// OutputModuleTypeUnprefixed returns the unprefixed output module's type so the type
// will **never** be prefixed with "proto:".
func (s *Sinker) OutputModuleTypeUnprefixed() (unprefixed string) {
	unprefixed, _ = sanitizeModuleType(s.outputModule.Output.Type)
	return
}

// ClientConfig returns the the `SubstreamsClientConfig`used by this sinker instance.
func (s *Sinker) ClientConfig() *client.SubstreamsClientConfig {
	return s.clientConfig
}

// EndpointConfig returns the endpoint configuration used by this sinker instance, this is an extraction
// of the `SubstreamsClientConfig` used by this sinker instance.
func (s *Sinker) EndpointConfig() (endpoint string, plaintext bool, insecure bool) {
	return s.clientConfig.Endpoint(), s.clientConfig.PlainText(), s.clientConfig.Insecure()
}

// ApiToken returns the currently defined ApiToken sets on this sinker instance, ""
// is no api token was configured
func (s *Sinker) ApiToken() string {
	return s.clientConfig.AuthToken()
}

func (s *Sinker) Run(ctx context.Context, cursor *Cursor, handler SinkerHandler) {
	s.OnTerminating(func(_ error) {
		s.logger.Info("sinker terminating")
		s.stats.Close()
	})
	s.stats.OnTerminated(func(err error) { s.Shutdown(err) })

	logEach := 15 * time.Second
	if s.logger.Core().Enabled(zap.DebugLevel) {
		logEach = 5 * time.Second
	}

	s.stats.Start(logEach)

	fields := []zap.Field{zap.Duration("stats_refresh_each", logEach)}
	if cursor != nil {
		fields = append(fields, zap.Stringer("restarting_at", cursor.Block()))
	}
	if s.adjustedEndBlock() != 0 {
		fields = append(fields, zap.String("end_at", fmt.Sprintf("#%d", s.adjustedEndBlock()-1)))
	}

	s.logger.Info("starting sinker", fields...)
	lastCursor, err := s.run(ctx, cursor, handler)
	if err == nil {
		s.logger.Info("substreams ended correctly, reached your stop block", zap.Stringer("last_block_seen", lastCursor.Block()))

		if v, ok := handler.(SinkerCompletionHandler); ok {
			s.logger.Info("substreams handler has completion callback defined, calling it")

			if err := v.HandleBlockRangeCompletion(ctx, lastCursor); err != nil {
				s.Shutdown(fmt.Errorf("sinker completion handler error: %w", err))
				return
			}
		}
	}

	// If the context is canceled and we are here, it we have stop running without any other error, so Shutdown without error,
	// we are not the cause of the error. We still shutdown so Sinker last stats is still printed.
	shutdownErr := err
	if ctx.Err() == context.Canceled {
		shutdownErr = nil
	}

	s.Shutdown(shutdownErr)
}

func (s *Sinker) run(ctx context.Context, cursor *Cursor, handler SinkerHandler) (activeCursor *Cursor, err error) {
	activeCursor = cursor

	ssClient, closeFunc, callOpts, headers, err := client.NewSubstreamsClient(s.clientConfig)

	if err != nil {
		return activeCursor, fmt.Errorf("new substreams client: %w", err)
	}
	s.OnTerminating(func(_ error) { closeFunc() })

	var headersArray []string

	if len(s.extraHeaders) > 0 || headers != nil {
		if headers == nil {
			headers = make(client.Headers)
		}

		for k, v := range parseHeaders(s.extraHeaders) {
			headers[k] = v
		}

		headersArray = make([]string, 0, len(headers)*2)
		for k, v := range parseHeaders(s.extraHeaders) {
			headersArray = append(headersArray, k, v)
		}

		for k, v := range headers {
			headersArray = append(headersArray, k, v)
		}
	}

	// We will wait at max approximatively 5m before dying
	backOff := s.backOff
	s.logger.Debug("configured default backoff", zap.String("back_off", fmt.Sprintf("%#v", backOff)))

	if !s.infiniteRetry {
		s.logger.Debug("configured backoff to stop after 15 retries")
		backOff = backoff.WithMaxRetries(backOff, 15)
	}

	backOff = backoff.WithContext(backOff, ctx)

	startBlock := s.startBlock
	stopBlock := s.adjustedEndBlock()

	for {
		s.request = &pbsubstreamsrpc.Request{
			StartBlockNum:   startBlock,
			StopBlockNum:    stopBlock,
			StartCursor:     activeCursor.String(),
			FinalBlocksOnly: s.finalBlocksOnly,
			Modules:         s.pkg.Modules,
			OutputModule:    s.outputModule.Name,
			ProductionMode:  s.mode == SubstreamsModeProduction,
			NoopMode:        s.NoopMode,
		}

		s.logger.Info("sending request", zap.String("start_block", fmt.Sprintf("%d", startBlock)), zap.String("stop_block", fmt.Sprintf("%d", stopBlock)))

		// Add extra headers if set
		streamCtx := ctx
		if len(headersArray) > 0 {
			streamCtx = metadata.AppendToOutgoingContext(streamCtx, headersArray...)
		}

		var receivedMessage bool
		activeCursor, receivedMessage, err = s.doRequest(streamCtx, activeCursor, s.request, ssClient, callOpts, handler)

		// If we received at least one message, we must reset the backoff
		if receivedMessage {
			backOff.Reset()
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				// We must assume that receiving an `io.EOF` means the stop block was reached. This is because
				// on network that can skips block number, it's possible that we requested to stop on a block
				// number that is no in the chain meaning we will receive `io.EOF` but the last seen block before
				// it is not our block number, we must have confidence in the Substreams provider to respect the
				// protocol
				return activeCursor, nil
			}

			if ctxErr := ctx.Err(); errors.Is(ctxErr, context.Canceled) {
				s.logger.Debug("substreams encountered an error but we are currently terminating, ignoring it", zap.Error(err))
				return activeCursor, nil
			}

			// Retryable or not, we increment the error counter in all those cases
			SubstreamsErrorCount.Inc()

			var retryableError *derr.RetryableError
			if errors.As(err, &retryableError) {
				s.logger.Error("substreams encountered a retryable error", zap.Error(retryableError.Unwrap()))

				sleepFor := backOff.NextBackOff()
				if sleepFor == backoff.Stop {
					return activeCursor, fmt.Errorf("%w: %w", ErrBackOffExpired, retryableError.Unwrap())
				}

				s.logger.Info("sleeping before re-connecting", zap.Duration("sleep", sleepFor))
				time.Sleep(sleepFor)
			} else {
				// Let's not wrap the error, it's not retryable to user will see directly his own error
				return activeCursor, err
			}
		}
	}
}

// When an undo buffer is used, we most finished +N block later than real
// stop block to ensure we accumulate enough blocks to assert "finality".
func (s *Sinker) adjustedEndBlock() (endBlock uint64) {
	if s.stopBlock == 0 {
		return 0
	}

	endBlock = s.stopBlock
	if s.buffer != nil {
		adjusted := endBlock + uint64(s.buffer.Capacity())
		s.logger.Debug("adjusted request end block for buffer", zap.Uint64("initial", endBlock), zap.Uint64("adjusted", adjusted))
		endBlock = adjusted
	}
	return
}

func (s *Sinker) doRequest(
	ctx context.Context,
	activeCursor *Cursor,
	req *pbsubstreamsrpc.Request,
	ssClient pbsubstreamsrpc.StreamClient,
	callOpts []grpc.CallOption,
	handler SinkerHandler,
) (
	*Cursor,
	bool,
	error,
) {
	s.logger.Debug("launching substreams request", zap.Int64("start_block", req.StartBlockNum), zap.Stringer("cursor", activeCursor))
	receivedMessage := false

	stream, err := ssClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return activeCursor, receivedMessage, retryable(fmt.Errorf("call sf.substreams.rpc.v2.Stream/Blocks: %w", err))
	}

	for {
		if s.tracer.Enabled() {
			s.logger.Debug("substreams waiting to receive message", zap.Stringer("cursor", activeCursor))
		}

		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return activeCursor, receivedMessage, err
			}

			if dgrpcError := dgrpc.AsGRPCError(err); dgrpcError != nil {
				switch dgrpcError.Code() {
				case codes.Unauthenticated:
					return activeCursor, receivedMessage, fmt.Errorf("stream failure: %w", err)

				case codes.InvalidArgument:
					return activeCursor, receivedMessage, fmt.Errorf("stream invalid: %w", err)

				}
			}

			if eh, ok := handler.(SinkerErrorHandler); ok {
				eh.HandleError(ctx, err)
			}

			return activeCursor, receivedMessage, retryable(err)
		}

		receivedMessage = true
		MessageSizeBytes.AddInt(proto.Size(resp))

		switch r := resp.Message.(type) {
		case *pbsubstreamsrpc.Response_Progress:
			if ph, ok := handler.(SinkerProgressHandler); ok {
				ph.HandleProgress(ctx, r.Progress)
			}

			msg := r.Progress

			latestEndBlockPerStage := make(map[uint32]uint64)
			jobsPerStage := make(map[uint32]uint64)

			for _, j := range msg.RunningJobs {
				jobEndBlock := j.StartBlock + j.ProgressBlocks
				if prevEndBlock, ok := latestEndBlockPerStage[j.Stage]; !ok || jobEndBlock > prevEndBlock {
					latestEndBlockPerStage[j.Stage] = jobEndBlock
				}
				jobsPerStage[j.Stage]++
			}
			for k, val := range latestEndBlockPerStage {
				ProgressMessageLastBlock.SetUint64(val, stageString(k))
			}
			for k, val := range jobsPerStage {
				ProgressMessageRunningJobs.SetUint64(val, stageString(k))
			}

			stagesModules := make(map[int][]string)
			for i, stage := range msg.Stages {
				stagesModules[i] = stage.Modules
				for j, r := range stage.CompletedRanges {
					if s.mode == SubstreamsModeProduction && i == len(msg.Stages)-1 { // last stage in production is a mapper. There may be "completed ranges" below the one that includes our start_block
						if s.requestActiveStartBlock <= r.StartBlock && r.EndBlock >= s.requestActiveStartBlock {
							ProgressMessageLastContiguousBlock.SetUint64(r.EndBlock, stageString(uint32(i)))
						}
					} else {
						if j == 0 {
							ProgressMessageLastContiguousBlock.SetUint64(r.EndBlock, stageString(uint32(i)))
						}
					}
				}
			}

			ProgressMessageCount.Inc()
			// The returned value from the server gives an overview of the current progress and not the delta
			// since the last message. Since the server is the source of truth, we just set the value directly.
			ProgressMessageTotalProcessedBlocks.SetUint64(r.Progress.ProcessedBlocks)
			ProgressMessageProcessedBytes.SetUint64(r.Progress.ProcessedBytes.TotalBytesRead)

			if s.tracer.Enabled() {
				s.logger.Debug("received response Progress", zap.Reflect("progress", r))
			}

		case *pbsubstreamsrpc.Response_BlockScopedData:
			block := bstream.NewBlockRef(r.BlockScopedData.Clock.Id, r.BlockScopedData.Clock.Number)
			moduleOutput := r.BlockScopedData.Output

			if s.tracer.Enabled() {
				s.logger.Debug("received response BlockScopedData", zap.Stringer("at", block), zap.String("module_name", moduleOutput.Name), zap.Int("payload_bytes", len(moduleOutput.MapOutput.Value)))
			}

			// We record our stats before the buffer action, so user sees state of "stream" and not state of buffer
			s.stats.RecordBlock(block)
			HeadBlockNumber.SetUint64(block.Num())
			HeadBlockTimeDrift.SetBlockTime(r.BlockScopedData.Clock.Timestamp.AsTime())
			DataMessageCount.Inc()
			DataMessageSizeBytes.AddInt(proto.Size(r.BlockScopedData))
			BackprocessingCompletion.SetUint64(1)

			cursor, err := NewCursor(r.BlockScopedData.Cursor)
			if err != nil {
				return activeCursor, receivedMessage, fmt.Errorf("invalid received cursor, 'bstream' library in here is probably not up to date: %w", err)
			}

			activeCursor = cursor

			var dataToProcess []*pbsubstreamsrpc.BlockScopedData
			if s.buffer == nil {
				// No buffering, process directly
				dataToProcess = []*pbsubstreamsrpc.BlockScopedData{r.BlockScopedData}
			} else {
				dataToProcess, err = s.buffer.HandleBlockScopedData(r.BlockScopedData)
				if err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("buffer add block data: %w", err)
				}
			}

			for _, blockScopedData := range dataToProcess {
				currentCursor, err := NewCursor(blockScopedData.Cursor)
				if err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("invalid received cursor, 'bstream' library in here is probably not up to date: %w", err)
				}

				var isLive *bool
				if s.livenessChecker != nil {
					isLive = &blockNotLive
					if s.livenessChecker.IsLive(blockScopedData.Clock) {
						isLive = &liveBlock
					}
				}

				if err := handler.HandleBlockScopedData(ctx, blockScopedData, isLive, currentCursor); err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("handle BlockScopedData message at block %s: %w", block, err)
				}
			}

		case *pbsubstreamsrpc.Response_BlockUndoSignal:
			undoSignal := r.BlockUndoSignal
			block := bstream.NewBlockRef(undoSignal.LastValidBlock.Id, undoSignal.LastValidBlock.Number)

			if s.tracer.Enabled() {
				s.logger.Debug("received response BlockUndoSignal", zap.Stringer("last_valid_block", block), zap.String("last_valid_cursor", undoSignal.LastValidCursor))
			}

			cursor, err := NewCursor(undoSignal.LastValidCursor)
			if err != nil {
				return activeCursor, receivedMessage, fmt.Errorf("invalid received cursor, 'bstream' library in here is probably not up to date: %w", err)
			}

			activeCursor = cursor

			// We record our stats before the buffer action, so user sees state of "stream" and not state of buffer
			s.stats.RecordBlock(block)
			UndoMessageCount.Inc()
			HeadBlockNumber.SetUint64(block.Num())
			// We don't have the block time in undo case for now, so we don't change it

			if s.buffer == nil {
				if err := handler.HandleBlockUndoSignal(ctx, r.BlockUndoSignal, activeCursor); err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("handle BlockUndoSignal: %w", err)
				}
			} else {
				// In the case of dealing with an undo buffer, it's expected that a fork will never
				// go beyong the first block in the buffer because if it does, `s.buffer.HandleBlockUndoSignal` here
				// returns an error.
				//
				// This means ultimately that we expect to never call the downstream `BlockUndoSignalHandler` function.
				err = s.buffer.HandleBlockUndoSignal(r.BlockUndoSignal)
				if err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("buffer undo block: %w", err)
				}
			}

		case *pbsubstreamsrpc.Response_DebugSnapshotData:
			if ss, ok := handler.(SinkerSnapshotHandler); ok {
				if err := ss.HandleInitialSnapshotData(ctx, r.DebugSnapshotData); err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("handle initial snapshot data: %w", err)
				}
			} else {
				s.logger.Warn("received debug snapshot message, there is no reason to receive those here", zap.Reflect("message", r))
			}
		case *pbsubstreamsrpc.Response_DebugSnapshotComplete:
			if ss, ok := handler.(SinkerSnapshotHandler); ok {
				if err := ss.HandleInitialSnapshotComplete(ctx, r.DebugSnapshotComplete); err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("handle initial snapshot complete: %w", err)
				}
			} else {
				s.logger.Warn("received debug snapshot message, there is no reason to receive those here", zap.Reflect("message", r))
			}

		case *pbsubstreamsrpc.Response_Session:
			if sh, ok := handler.(SinkerSessionInitHandler); ok {
				if err := sh.HandleSessionInit(ctx, r.Session); err != nil {
					return activeCursor, receivedMessage, fmt.Errorf("handle session init: %w", err)
				}
				break
			}
			s.logger.Info("session initialized with remote endpoint",
				zap.Uint64("max_parallel_workers", r.Session.MaxParallelWorkers),
				zap.Uint64("linear_handoff_block", r.Session.LinearHandoffBlock),
				zap.Uint64("resolved_start_block", r.Session.ResolvedStartBlock),
				zap.String("trace_id", r.Session.TraceId),
			)
			s.requestActiveStartBlock = r.Session.ResolvedStartBlock

		default:
			s.logger.Info("received unknown type of message", zap.Reflect("message", r))
			UnknownMessageCount.Inc()
		}
	}
}

func stageString(i uint32) string {
	return fmt.Sprintf("stage %d", i)
}

func retryable(err error) error {
	return derr.NewRetryableError(err)
}

var (
	liveBlock    bool = true
	blockNotLive bool = false
)

func parseHeaders(headers []string) map[string]string {
	if headers == nil {
		return nil
	}

	result := make(map[string]string)
	for _, header := range headers {
		parts := strings.Split(header, ":")
		if len(parts) != 2 {
			log.Fatalf("invalid header format: %s", header)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
}
