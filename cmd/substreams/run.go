package main

import (
	"context"
	"fmt"

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
	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		s := <-derr.SetupSignalHandler(0)
		fmt.Println("received", s.String())
		cancelCause(fmt.Errorf("received signal %q", s.String()))
	}()

	ui.Connecting()
	sinker.Run(ctx, cursor, h)
	err = sinker.Err()

	ui.Cancel()
	sinker.PrintStats()
	if err == nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		fmt.Println("Completed successfully")
		return nil
	}

	return err
}
