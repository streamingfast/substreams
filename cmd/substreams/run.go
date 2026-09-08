package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jhump/protoreflect/dynamic"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tools/test"
	"github.com/streamingfast/substreams/tui"
	"github.com/streamingfast/substreams/tui2/common"
	"go.uber.org/zap"
)

func init() {
	sink.AddFlagsToSet(runCmd.Flags(),
		sink.FlagIncludeOptional(
			sink.FlagCursor,
			sink.FlagPartialBlocks,
		),
		sink.FlagExcludeDefault(
			sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
		),
	)

	runCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	runCmd.Flags().Uint64("limit-processed-blocks", 10000, "Limit the number of blocks to be processed by the server, including preparing the stores, as a safeguard to prevent unexpected expensive reprocessing (0 disables the limit)")
	runCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode)")

	runCmd.Flags().StringP("output", "o", "", "Output mode, one of: [tui (and ui), json, jsonl, clock] Defaults to 'tui' when in a TTY is present, and 'json' otherwise")
	runCmd.Flags().String("bytes-encoding", "", "Encoding to use for all bytes representation, one of: ['', 'hex', 'base58', 'base64', 'string']. If empty, will guess based on the network, falling back to hex")

	runCmd.Flags().String("test-file", "", "Runs a test file")
	runCmd.Flags().Bool("test-verbose", false, "Print out all the results")

	rootCmd.AddCommand(runCmd)
}

// runCmd represents the command to run substreams remotely
var runCmd = &cobra.Command{
	Use:          "run [<manifest> [<module_name>]]",
	Short:        "Stream module to standard output. Use 'substreams gui' for more tools and a better experience.",
	Long:         guiOrRunLongUsage,
	RunE:         runRun,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func runRun(cmd *cobra.Command, args []string) error {
	// note:  zlog in CLI only prints Warn/Err, not Info by default
	ctx := cmd.Context()

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

	// Load auth environment file if it exists
	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	// parses flags
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "substreams_run", zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sink config: %w", err)
	}

	// override from our own production-mode flag
	if sflags.MustGetBool(cmd, "production-mode") {
		sinkerConfig.Mode = sink.SubstreamsModeProduction
	} else {
		if sinkerConfig.NoopMode {
			zlog.Warn("noop-mode used without production-mode: server will execute in development mode without sending the data, this is probably not what you want")
		}
		sinkerConfig.Mode = sink.SubstreamsModeDevelopment
	}

	outputModulesSnapshot := sflags.MustGetStringSlice(cmd, "debug-modules-initial-snapshot")
	if len(outputModulesSnapshot) != 0 {
		sinkerConfig.DevOutputSnapshots = outputModulesSnapshot
	}

	sinkerConfig.LimitProcessedBlocks = sflags.MustGetUint64(cmd, "limit-processed-blocks")

	sinker, err := sink.NewFromConfig(sinkerConfig)
	if err != nil {
		return fmt.Errorf("creating sink: %w", err)
	}

	cursorStr := sflags.MustGetString(cmd, "cursor")
	cursor, err := sink.NewCursor(cursorStr)
	if err != nil {
		return fmt.Errorf("creating cursor: %w", err)
	}

	outputMode := sflags.MustGetString(cmd, "output")
	ui, err := tui.New(
		sinker.Package(),
		[]string{outputModule},
	)
	if err != nil {
		return fmt.Errorf("creating ui: %w", err)
	}

	endpoint, _, _ := sinker.EndpointConfig()
	ui.SetEndpoint(endpoint)

	testFile := sflags.MustGetString(cmd, "test-file")
	if testFile != "" {
		msgDescs, err := manifest.BuildMessageDescriptors(sinker.Package())
		if err != nil {
			return fmt.Errorf("building message descriptors: %w", err)
		}
		zlog.Info("running test runner", zap.String(testFile, testFile))
		testRunner, err := test.NewRunner(testFile, msgDescs, sflags.MustGetBool(cmd, "test-verbose"), zlog)
		if err != nil {
			return fmt.Errorf("failed to setup test runner: %w", err)
		}
		ui.SetTestRunner(testRunner)
	}

	var bytesRepresentation dynamic.BytesRepresentation

	enc := sflags.MustGetString(cmd, "bytes-encoding")
	switch enc {
	case "hex":
		bytesRepresentation = dynamic.BytesAsHex
	case "base64":
		bytesRepresentation = dynamic.BytesAsBase64
	case "base58":
		bytesRepresentation = dynamic.BytesAsBase58
	case "string":
		bytesRepresentation = dynamic.BytesAsString
	case "":
		bytesRepresentation = common.ToDynamicBytesRepresentation(sinker.BytesRepresentation())
	default:
		return fmt.Errorf("invalid bytes encoding: %s", enc)
	}

	if err := ui.Init(outputMode, bytesRepresentation); err != nil {
		return fmt.Errorf("TUI initialization: %w", err)
	}
	defer ui.CleanUpTerminal()

	h := sink.NewSinkerFullHandlersWithPartial(
		ui.HandleBlockScopedData,
		ui.HandleBlockUndoSignal,
		ui.HandleSession,
		ui.HandleProgress,
		ui.HandleDebugSnapshotData,
		ui.HandleDebugSnapshotComplete,
		ui.HandleError,
	)
	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		s := <-derr.SetupSignalHandler(0)
		fmt.Fprintln(os.Stderr, "received", s.String())
		cancelCause(fmt.Errorf("received signal %q", s.String()))
	}()

	// While the live view holds the terminal it swallows Ctrl-C, so the signal handler above
	// never fires and the request has to be cancelled from the key press instead.
	ui.SetInterruptHandler(func() { cancelCause(errors.New("interrupted by user")) })

	ui.Connecting()
	sinker.Run(ctx, cursor, h)
	err = sinker.Err()

	ui.Cancel()
	ui.AbortProgress()

	// The progress block, the usage report and the error each come from a different writer and
	// otherwise pile up as one wall of text.
	fmt.Fprintln(os.Stderr)
	sinker.PrintStats()
	fmt.Fprintln(os.Stderr)

	if err == nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		fmt.Fprintln(os.Stderr, "Completed successfully")
		return nil
	}

	return explainRunError(err, ui, sflags.MustGetUint64(cmd, "limit-processed-blocks"))
}
