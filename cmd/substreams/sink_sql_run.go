package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

var sinkSQLRunCmd = &cobra.Command{
	Use:     "run <manifest> [<start>:<stop>]",
	Short:   "Runs SQL sink process",
	Example: "substreams sink sql run uniswap-v3@v0.2.10 --dsn 'postgres://localhost:5432/postgres?sslmode=disable'",
	Args:    cobra.RangeArgs(1, 2),
	RunE:    sinkSQLRunE,
}

func init() {
	flags := sinkSQLRunCmd.Flags()
	sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addCommonSinkerFlags(flags)
	addCommonDatabaseChangesFlags(flags)

	flags.Int("undo-buffer-size", 0, "If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database (required for some databases like Postgres).")
	flags.Int("batch-block-flush-interval", 1_000, "When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("batch-row-flush-interval", 100_000, "When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks.")
	flags.Int("flush-interval", 0, "(deprecated) please use --batch-block-flush-interval instead")
	flags.Int("flush-retry-count", 3, "Number of retry attempts for flush operations")
	flags.Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures")

	sinkSQLCmd.AddCommand(sinkSQLRunCmd)
}

func sinkSQLRunE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	sinkSQLPreStart(cmd)

	app := cli.NewApplication(cmd.Context())

	sinker.RegisterMetrics()

	dsnString, err := resolveSinkSQLDSN(cmd)
	if err != nil {
		return err
	}

	manifestPath := args[0]

	if len(args) > 1 {
		blockRangeArg := args[1]
		if strings.Contains(blockRangeArg, ":") {
			br, err := readBlockRangeArgument(blockRangeArg)
			if err != nil {
				return fmt.Errorf("invalid block range %q: %w", blockRangeArg, err)
			}

			if br.StartBlock() > 0 {
				if err := cmd.Flags().Set("start-block", fmt.Sprintf("%d", br.StartBlock())); err != nil {
					return fmt.Errorf("setting start-block flag: %w", err)
				}
			}
			if br.EndBlock() != nil {
				if err := cmd.Flags().Set("stop-block", fmt.Sprintf("%d", *br.EndBlock())); err != nil {
					return fmt.Errorf("setting stop-block flag: %w", err)
				}
			}
		}
	}

	baseSink, err := sink.NewFromViper(
		cmd,
		supportedOutputTypes,
		manifestPath,
		sink.InferOutputModuleFromPackage,
		"sink_sql",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	batchBlockFlushInterval := sflags.MustGetInt(cmd, "batch-block-flush-interval")
	if sflags.MustGetInt(cmd, "flush-interval") != 0 {
		batchBlockFlushInterval = sflags.MustGetInt(cmd, "flush-interval")
	}

	sinkerFactory := sinker.SinkerFactory(baseSink, sinker.SinkerFactoryOptions{
		CursorTableName:         sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:        sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:       sflags.MustGetString(cmd, "clickhouse-cluster"),
		BatchBlockFlushInterval: batchBlockFlushInterval,
		BatchRowFlushInterval:   sflags.MustGetInt(cmd, "batch-row-flush-interval"),
		LiveBlockFlushInterval:  sflags.MustGetInt(cmd, "live-block-flush-interval"),
		OnModuleHashMismatch:    resolveOnModuleHashMismatchFlag(cmd),
		HandleReorgs:            sflags.MustGetInt(cmd, "undo-buffer-size") == 0,
		FlushRetryCount:         sflags.MustGetInt(cmd, "flush-retry-count"),
		FlushRetryDelay:         sflags.MustGetDuration(cmd, "flush-retry-delay"),
	})

	sqlSinker, err := sinkerFactory(app.Context(), dsnString, zlog, tracer)
	if err != nil {
		return fmt.Errorf("unable to setup sql sinker: %w", err)
	}

	app.SuperviseAndStart(sqlSinker)

	return app.WaitForTermination(zlog, 0*time.Second, 30*time.Second)
}
