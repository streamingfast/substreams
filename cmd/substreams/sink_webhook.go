package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func init() {

	// default sinker flags
	sink.AddFlagsToSet(sinkWebhookCmd.Flags(),
		sink.FlagIgnore(sink.FlagDevelopmentMode,
			sink.FlagLiveBlockTimeDelta,
			sink.FlagInfiniteRetry))

	sinkWebhookCmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor. If empty, no cursor will be saved or used, only the start-block.")

	SinkCmd.AddCommand(sinkWebhookCmd)
}

// runCmd represents the command to run substreams remotely
var sinkWebhookCmd = &cobra.Command{
	Use:   "webhook <url> [<manifest> [<module_name>]]",
	Short: "Trigger a webhook call for each event from a substreams module",
	RunE:  sinkWebhookE,
	Args:  cobra.RangeArgs(1, 3),
}

func sinkWebhookE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	url := args[0]

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	// parses flags
	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "substreams_webhook", zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sink config: %w", err)
	}

	sinker := sink.New(sinkerConfig)

	decoder, err := protodecode.NewDecoder(sinker.Package(), []string{sinker.OutputModuleName()})
	if err != nil {
		return fmt.Errorf("creating decoder: %w", err)
	}

	stateFileStr := sflags.MustGetString(cmd, "state-file")
	_ = stateFileStr //FIXME cursor handling

	h := sink.NewSinkerHandlers( //FIXME get those from sink subfolder

		func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
			if data.Output.MapOutput.Value == nil {
				return nil
			}

			//msgType := decoder.GetMessageType(data.Output.Name)
			msgDesc := decoder.GetMessageDescriptor(data.Output.Name)
			dataContent := decoder.DecodeDynamicMessage(msgDesc, data.Output.MapOutput)

			//wrappedOut, err := decoder.WrapMessage(msgType, data.Clock.Number, data.Output.Name, dataContent)
			//if err != nil {
			//	return fmt.Errorf("failed to wrap message: %w", err)
			//}

			fmt.Println("would call webhook for block", url, data.Clock.Number, "with data", string(dataContent))
			return nil
		},
		func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
			// FIXME: save new cursor
			return nil
		},
	)

	sinker.Run(ctx, nil, h)
	err = sinker.Err()

	fmt.Fprintf(os.Stderr, "Total Processed Bytes: %d\n", uint64(sink.ProgressMessageProcessedBytes.Get()))
	fmt.Fprintf(os.Stderr, "Total Processed Blocks: %d\n", uint64(sink.ProgressMessageTotalProcessedBlocks.Get()))
	fmt.Fprintf(os.Stderr, "Total Received Bytes (uncompressed gress): %d\n", uint64(sink.DataMessageSizeBytes.Get()))
	fmt.Fprintln(os.Stderr, "all done")

	return err
}
