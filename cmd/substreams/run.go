package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/tools/test"
	"github.com/streamingfast/substreams/tui"
	"github.com/streamingfast/substreams/tui2/common"
	"go.uber.org/zap"
)

func init() {

	// default sinker flags
	sink.AddFlagsToSet(runCmd.Flags(),
		sink.FlagIgnore(sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagInfiniteRetry))

	runCmd.Flags().Bool("production-mode", false, "Enable Production Mode, with high-speed parallel processing")
	runCmd.Flags().Uint64("limit-processed-blocks", 10000, "Limit the number of blocks to be processed by the server, including preparing the stores, as a safeguard to prevent unexpected expensive reprocessing (0 disables the limit)")
	runCmd.Flags().StringSlice("debug-modules-initial-snapshot", nil, "List of 'store' modules from which to print the initial data snapshot (Unavailable in Production Mode)")
	runCmd.Flags().StringSlice("debug-modules-output", nil, "List of modules from which to print outputs, deltas and logs, accepts regexes (Unavailable in Production Mode)")

	runCmd.Flags().StringP("output", "o", "", "Output mode, one of: [tui (and ui), json, jsonl, clock] Defaults to 'tui' when in a TTY is present, and 'json' otherwise")

	runCmd.Flags().String("test-file", "", "runs a test file")
	runCmd.Flags().Bool("test-verbose", false, "print out all the results")

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
	ctx := cmd.Context()

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

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

	sinkerConfig.DevOutputModules = sflags.MustGetStringSlice(cmd, "debug-modules-output")
	if sinkerConfig.DevOutputModules != nil && sinkerConfig.Mode == sink.SubstreamsModeProduction {
		return fmt.Errorf("cannot set 'debug-modules-output' in 'production-mode'")
	}

	outputModulesSnapshot := sflags.MustGetStringSlice(cmd, "debug-modules-initial-snapshot")
	if len(outputModulesSnapshot) != 0 {
		sinkerConfig.DevOutputSnapshots = outputModulesSnapshot
	}

	sinkerConfig.LimitProcessedBlocks = sflags.MustGetUint64(cmd, "limit-processed-blocks")

	sinker := sink.New(sinkerConfig)

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

	if err := ui.Init(outputMode, common.ToDynamicBytesRepresentation(sinker.BytesRepresentation())); err != nil {
		return fmt.Errorf("TUI initialization: %w", err)
	}
	defer ui.CleanUpTerminal()

	h := sink.NewSinkerFullHandlers(
		ui.HandleBlockScopedData,
		ui.HandleBlockUndoSignal,
		ui.HandleSession,
		ui.HandleProgress,
		ui.HandleDebugSnapshotData,
		ui.HandleDebugSnapshotComplete,
		nil,
	)

	ui.Connecting()
	sinker.Run(ctx, cursor, h)
	err = sinker.Err()

	ui.Cancel()
	fmt.Fprintf(os.Stderr, "Total Processed Bytes: %d\n", uint64(sink.ProgressMessageProcessedBytes.Get()))
	fmt.Fprintf(os.Stderr, "Total Processed Blocks: %d\n", uint64(sink.ProgressMessageTotalProcessedBlocks.Get()))
	fmt.Fprintf(os.Stderr, "Total Received Bytes (uncompressed gress): %d\n", uint64(sink.DataMessageSizeBytes.Get()))
	fmt.Fprintln(os.Stderr, "all done")

	return err

	//	if readFromModule {
	//		sb, err := pkgBundle.Graph.ModuleInitialBlock(outputModule)
	//		if err != nil {
	//			return fmt.Errorf("getting module start block: %w", err)
	//		}
	//		startBlock = int64(sb)
	//	}
	//
	// FIXME: provide the user_agent for the client...
	//
	// FIXME

}
