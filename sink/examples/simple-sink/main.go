package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func init() {
	// Add the list of common sink flags from the sink package, skipping the ones you don't want or want to define yourself
	sink.AddFlagsToSet(cmd.Flags(),
		sink.FlagIgnore(
			sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagInfiniteRetry,
		))

	// add your own flags, like this one for state management
	cmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor. If empty, no cursor will be saved or used, only the start-block.")
}

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var cmd = &cobra.Command{
	Use:   "simple-sink [<manifest> [<module_name>]]",
	Short: "Trigger a webhook call for each event from a substreams module",
	RunE:  runE,
	Args:  cobra.RangeArgs(0, 2),
}

// Initialize logging and tracing for the sink followwing Streamingfast's logging pattern
var zlog, tracer = logging.RootLogger("simple-sink", "github.com/streamingfast/substreams/sink/example/simple-sink")

// Add our own metrics to the ones exposed in the sink
var EventCounter = sink.Metrics.NewCounter("simple_sink_events", "Number of events processed by the simple sink")

func runE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// parse positional arguments
	var manifestPath string
	var outputModule string
	switch len(args) {
	case 1:
		manifestPath = args[0]
	case 2:
		manifestPath = args[0]
		outputModule = args[1]
	}

	// Load auth environment file if it exists
	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	// parses flags
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "simple_sink", zlog, tracer)
	if err != nil {
		return err
	}

	// Get your state file configuration from flags
	stateFile := sflags.MustGetString(cmd, "state-file")

	initialCursor, err := sink.ReadCursor(stateFile)
	if err != nil {
		return err
	}

	mySink := &SimpleSink{
		stateFile:    stateFile,
		sinkerConfig: sinkerConfig,
	}

	return mySink.Run(ctx, initialCursor)
}

type SimpleSink struct {
	stateFile    string
	sinkerConfig *sink.SinkerConfig
	decoder      *protodecode.Decoder
}

func (s *SimpleSink) Run(ctx context.Context, initialCursor *sink.Cursor) error {
	// set up dynamic protobuf decoder to JSON
	decoder, err := protodecode.NewDecoder(s.sinkerConfig.Pkg, []string{s.sinkerConfig.OutputModule.Name})
	if err != nil {
		return fmt.Errorf("getting decoder: %w", err)
	}
	s.decoder = decoder

	// run the actual sinker that will call our Handle methods
	sinker := sink.New(s.sinkerConfig)
	sinker.Run(
		ctx,
		initialCursor,
		s,
	)

	sinker.PrintStats() // print usage statistics

	return nil
}

// handleBlockScopedData processes the data from a block
func (s *SimpleSink) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	// skip empty data (when live, only clock is sent)
	if data.Output.MapOutput.Value == nil {
		return nil
	}

	EventCounter.AddInt(1)

	msgDesc := s.decoder.GetMessageDescriptor(data.Output.Name)
	dataContent := s.decoder.DecodeDynamicMessage(msgDesc, data.Output.MapOutput)

	// Do something with the decoded data, ex: write to a DB or file
	fmt.Printf("block %d (%s) data: %q", data.Clock.Number, data.Clock.Id, dataContent)

	// Save cursor to state file
	if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
		return err
	}

	return nil
}

// handleBlockUndoSignal handles undo signals
func (s *SimpleSink) HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {

	// Do something with the information about the UNDO event (chain reorg)
	// ex: rollback DB changes
	fmt.Printf("UNDO block up to %d (%s)", undoSignal.LastValidBlock.Number, undoSignal.LastValidBlock.Id)

	// Save cursor to state file
	if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
		return err
	}
	return nil
}
