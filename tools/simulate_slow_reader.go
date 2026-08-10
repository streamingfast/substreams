package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
)

var simulateSlowReaderCmd = &cobra.Command{
	Use:   "simulate-slow-reader <manifest> [<module_name>]",
	Short: "Consume a substreams deliberately slowly, to exercise server-side back-pressure",
	Long: ExamplePrefixed("substreams tools simulate-slow-reader", `
		Streams a substreams and waits before handling each block, so the server ends up blocked
		writing to this client.

		The wait blocks the receive loop rather than happening in the background, which is what a
		genuinely slow consumer does: the gRPC flow-control window fills and the server's SendMsg
		blocks. That is what the server reports in its periodic "substreams request progress" log,
		as the time it spent blocked writing to the consumer.

		Nothing is decoded or printed per block: this is a load-shaping tool, not a way to look at
		data. Use 'substreams run' for that.
	`),
	RunE:         simulateSlowReaderE,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func init() {
	sink.AddFlagsToSet(simulateSlowReaderCmd.Flags(),
		sink.FlagIncludeOptional(sink.FlagCursor),
		sink.FlagExcludeDefault(sink.FlagDevelopmentMode, sink.FlagLiveBlockTimeDelta),
	)

	simulateSlowReaderCmd.Flags().Duration("delay", time.Second, "How long to wait before handling each received block")
	simulateSlowReaderCmd.Flags().Bool("production-mode", true, "Enable Production Mode, with high-speed parallel processing")
	simulateSlowReaderCmd.Flags().Uint64("limit-processed-blocks", 0, "Limit the number of blocks the server may process, 0 disables the limit")

	Cmd.AddCommand(simulateSlowReaderCmd)
}

func simulateSlowReaderE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	manifestPath := args[0]
	var outputModule string
	if len(args) == 2 {
		outputModule = args[1]
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "substreams_simulate_slow_reader", zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sink config: %w", err)
	}

	sinkerConfig.Mode = sink.SubstreamsModeDevelopment
	if sflags.MustGetBool(cmd, "production-mode") {
		sinkerConfig.Mode = sink.SubstreamsModeProduction
	}
	sinkerConfig.LimitProcessedBlocks = sflags.MustGetUint64(cmd, "limit-processed-blocks")

	sinker, err := sink.NewFromConfig(sinkerConfig)
	if err != nil {
		return fmt.Errorf("creating sink: %w", err)
	}

	cursor, err := sink.NewCursor(sflags.MustGetString(cmd, "cursor"))
	if err != nil {
		return fmt.Errorf("creating cursor: %w", err)
	}

	delay := sflags.MustGetDuration(cmd, "delay")
	zlog.Info("reading deliberately slowly", zap.Duration("delay_per_block", delay), zap.String("output_module", sinkerConfig.OutputModule.GetName()))

	var blocks uint64
	started := time.Now()
	handleBlockScopedData := func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
		// Blocking here is the point: it stops reading from the stream, which is what exerts
		// back-pressure on the server. Sleeping in a goroutine would exert none.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		blocks++
		if blocks%10 == 0 {
			zlog.Info("still reading slowly",
				zap.Uint64("blocks_read", blocks),
				zap.Uint64("at_block", data.Clock.GetNumber()),
				zap.Duration("elapsed", time.Since(started).Round(time.Second)),
			)
		}
		return nil
	}

	handler := sink.NewSinkerHandlers(handleBlockScopedData, func(context.Context, *pbsubstreamsrpc.BlockUndoSignal, *sink.Cursor) error {
		return nil
	})

	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		s := <-derr.SetupSignalHandler(0)
		cancelCause(fmt.Errorf("received signal %q", s.String()))
	}()

	sinker.Run(ctx, cursor, handler)

	zlog.Info("done", zap.Uint64("blocks_read", blocks), zap.Duration("elapsed", time.Since(started).Round(time.Second)))
	if err := sinker.Err(); err != nil {
		return err
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return nil
}
