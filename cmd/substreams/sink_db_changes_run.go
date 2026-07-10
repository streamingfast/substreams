package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

var sinkDBChangesRunCmd = &cobra.Command{
	Use:     "run <manifest> [<start>:<stop>]",
	Short:   "Runs the DatabaseChanges SQL sink process",
	Example: "substreams sink database-changes run uniswap-v3@v0.2.10 --dsn 'postgres://localhost:5432/postgres?sslmode=disable'",
	Args:    cobra.RangeArgs(1, 2),
	RunE:    sinkDBChangesRunE,
}

func init() {
	flags := sinkDBChangesRunCmd.Flags()
	sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addCommonSinkerFlags(flags)
	addCommonDatabaseChangesFlags(flags)

	flags.Int("undo-buffer-size", 0, "If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database (required for some databases like Postgres).")
	flags.Int("batch-block-flush-interval", 1_000, "When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("batch-row-flush-interval", 100_000, "When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks.")
	flags.Int("flush-retry-count", 3, "Number of retry attempts for flush operations")
	flags.Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures")

	sinkDBChangesCmd.AddCommand(sinkDBChangesRunCmd)
}

func sinkDBChangesRunE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	manifestPath := args[0]

	if len(args) > 1 {
		if err := applyBlockRangeArg(cmd, args[1]); err != nil {
			return err
		}
	}

	sinkDBPreStart(cmd)

	app := cli.NewApplication(cmd.Context())

	sinker.RegisterMetrics()

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	baseSink, err := sink.NewFromViper(
		cmd,
		supportedOutputTypes,
		manifestPath,
		sink.InferOutputModuleFromPackage,
		"sink_database_changes",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	sinkerFactory := sinker.SinkerFactory(baseSink, sinker.SinkerFactoryOptions{
		CursorTableName:         sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:        sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:       sflags.MustGetString(cmd, "clickhouse-cluster"),
		BatchBlockFlushInterval: sflags.MustGetInt(cmd, "batch-block-flush-interval"),
		BatchRowFlushInterval:   sflags.MustGetInt(cmd, "batch-row-flush-interval"),
		LiveBlockFlushInterval:  sflags.MustGetInt(cmd, "live-block-flush-interval"),
		OnModuleHashMismatch:    sflags.MustGetString(cmd, onModuleHashMismatchFlag),
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
