package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"

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

	webhookURL := args[0]

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

	// Load existing cursor if state file exists
	var startCursor *sink.Cursor
	if stateFileStr != "" {
		if data, err := ioutil.ReadFile(stateFileStr); err == nil {
			cursorStr := strings.TrimSpace(string(data))
			if cursorStr != "" {
				if cursor, err := sink.NewCursor(cursorStr); err == nil {
					startCursor = cursor
					zlog.Info("loaded cursor from state file", zap.String("cursor", cursorStr), zap.String("file", stateFileStr))
				} else {
					zlog.Warn("failed to parse cursor from state file", zap.Error(err), zap.String("file", stateFileStr))
				}
			}
		}
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	h := sink.NewSinkerHandlers(

		func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
			if data.Output.MapOutput.Value == nil {
				return nil
			}

			msgDesc := decoder.GetMessageDescriptor(data.Output.Name)
			dataContent := decoder.DecodeDynamicMessage(msgDesc, data.Output.MapOutput)

			wrappedOut, err := decoder.WrapMessage(data.Output.MapOutput.TypeUrl, data.Clock.Number, data.Output.Name, dataContent)
			if err != nil {
				return fmt.Errorf("failed to wrap message: %w", err)
			}

			zlog.Info("calling webhook", zap.Uint64("block", data.Clock.Number), zap.String("url", webhookURL))

			// Make the webhook call
			req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(wrappedOut))
			if err != nil {
				zlog.Warn("failed to create webhook request", zap.Error(err), zap.Uint64("block", data.Clock.Number))
				return nil // Continue processing
			}

			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				zlog.Warn("webhook call failed", zap.Error(err), zap.Uint64("block", data.Clock.Number), zap.String("url", webhookURL))
				return nil // Continue processing
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				zlog.Warn("webhook returned non-success status", zap.Int("status", resp.StatusCode), zap.Uint64("block", data.Clock.Number), zap.String("url", webhookURL))
				return nil // Continue processing
			}

			// Save cursor to state file
			if stateFileStr != "" && cursor != nil {
				cursorStr := cursor.String()
				if err := ioutil.WriteFile(stateFileStr, []byte(cursorStr), 0644); err != nil {
					zlog.Warn("failed to save cursor to state file", zap.Error(err), zap.String("file", stateFileStr))
				}
			}
			return nil
		},
		func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
			// Save cursor to state file on undo
			if stateFileStr != "" && cursor != nil {
				cursorStr := cursor.String()
				if err := ioutil.WriteFile(stateFileStr, []byte(cursorStr), 0644); err != nil {
					zlog.Warn("failed to save cursor to state file on undo", zap.Error(err), zap.String("file", stateFileStr))
				}
			}
			return nil
		},
	)

	sinker.Run(ctx, startCursor, h)
	err = sinker.Err()

	fmt.Fprintf(os.Stderr, "Total Processed Bytes: %d\n", uint64(sink.ProgressMessageProcessedBytes.Get()))
	fmt.Fprintf(os.Stderr, "Total Processed Blocks: %d\n", uint64(sink.ProgressMessageTotalProcessedBlocks.Get()))
	fmt.Fprintf(os.Stderr, "Total Received Bytes (uncompressed gress): %d\n", uint64(sink.DataMessageSizeBytes.Get()))
	fmt.Fprintln(os.Stderr, "all done")

	return err
}
