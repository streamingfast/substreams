package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/noop"
)

func init() {
	// default sinker flags - ignore specific flags for noop sink
	sink.AddFlagsToSet(sinkNoopCmd.Flags(),
		sink.FlagIgnore(
			sink.FlagUndoBufferSize,
			sink.FlagDevelopmentMode,
			sink.FlagNoopMode,
			sink.FlagProtoPath,
			sink.FlagProtoDescriptorSet,
		))

	sinkNoopCmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor. If empty, no cursor will be saved or used, only the start-block.")

	SinkCmd.AddCommand(sinkNoopCmd)
}

// sinkNoopCmd represents the command to run substreams noop sink
var sinkNoopCmd = &cobra.Command{
	Use:   "noop [<manifest> [<module_name>]]",
	Short: "Run a noop sink that only handles cursors without processing data",
	RunE:  sinkNoopE,
	Args:  cobra.RangeArgs(0, 2),
}

func sinkNoopE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}

	// Load auth environment file if it exists
	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	// parses flags
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink_noop", zlog, tracer)
	if err != nil {
		return err
	}

	// Force noop mode to true for noop sink
	sinkerConfig.NoopMode = true

	// Get state file configuration from flags
	stateFileStr := sflags.MustGetString(cmd, "state-file")

	// Create noop sink configuration
	sinkConfig := noop.SinkConfig{
		StateFile:    stateFileStr,
		SinkerConfig: sinkerConfig,
		Logger:       zlog,
	}

	// Create and run the noop sink
	noopSink, err := noop.NewSink(sinkConfig)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	err = noopSink.Run(ctx)

	// Print final statistics
	noopSink.PrintStats()

	return err
}
