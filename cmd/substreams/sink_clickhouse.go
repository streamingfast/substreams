package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	sfcli "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	sinker2 "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

func init() {
	clickhouseCmd.AddCommand(clickhouseRunCmd)
	clickhouseCmd.AddCommand(clickhouseSetupCmd)
	SinkCmd.AddCommand(clickhouseCmd)
}

var clickhouseCmd = &cobra.Command{
	Use:   "clickhouse",
	Short: "ClickHouse sink commands",
}

var clickhouseRunCmd = sfcli.Command(clickhouseRunE,
	"run <dsn> <manifest> [<start>:<stop>]",
	"Runs ClickHouse sink process",
	sfcli.RangeArgs(2, 3),
	sfcli.Flags(func(flags *pflag.FlagSet) {
		sink.AddFlagsToSet(flags, sink.FlagExcludeDefault("undo-buffer-size"))
		sqlAddCommonSinkerFlags(flags)
		sqlAddCommonDatabaseChangesFlags(flags)
		flags.Int("undo-buffer-size", 0, "If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database.")
		flags.Int("batch-block-flush-interval", 1_000, "When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval.")
		flags.Int("batch-row-flush-interval", 100_000, "When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval.")
		flags.Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks.")
		flags.Int("flush-retry-count", 3, "Number of retry attempts for flush operations.")
		flags.Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures.")
	}),
	sfcli.OnCommandErrorLogAndExit(zlog),
)

func clickhouseRunE(cmd *cobra.Command, args []string) error {
	app := sfcli.NewApplication(cmd.Context())

	sinker2.RegisterMetrics()

	dsnString := args[0]
	manifestPath := args[1]

	if len(args) > 2 {
		thirdArg := args[2]
		if strings.Contains(thirdArg, ":") {
			if err := sqlSetBlockRangeFlags(cmd, thirdArg); err != nil {
				return fmt.Errorf("invalid block range %q: %w", thirdArg, err)
			}
		}
	}

	baseSink, err := sink.NewFromViper(
		cmd,
		sqlSupportedOutputTypes,
		manifestPath,
		sink.InferOutputModuleFromPackage,
		fmt.Sprintf("substreams-sink-sql/%s", version),
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	batchBlockFlushInterval := sflags.MustGetInt(cmd, "batch-block-flush-interval")
	batchRowFlushInterval := sflags.MustGetInt(cmd, "batch-row-flush-interval")
	liveBlockFlushInterval := sflags.MustGetInt(cmd, "live-block-flush-interval")
	flushRetryCount := sflags.MustGetInt(cmd, "flush-retry-count")
	flushRetryDelay := sflags.MustGetDuration(cmd, "flush-retry-delay")

	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")
	handleReorgs := sflags.MustGetInt(cmd, "undo-buffer-size") == 0

	sinkerFactory := sinker2.SinkerFactory(baseSink, sinker2.SinkerFactoryOptions{
		CursorTableName:         cursorTableName,
		HistoryTableName:        historyTableName,
		ClickhouseCluster:       sflags.MustGetString(cmd, "clickhouse-cluster"),
		BatchBlockFlushInterval: batchBlockFlushInterval,
		BatchRowFlushInterval:   batchRowFlushInterval,
		LiveBlockFlushInterval:  liveBlockFlushInterval,
		OnModuleHashMismatch:    sqlResolveModuleHashMismatchFlag(cmd),
		HandleReorgs:            handleReorgs,
		FlushRetryCount:         flushRetryCount,
		FlushRetryDelay:         flushRetryDelay,
	})

	sqlSinker, err := sinkerFactory(app.Context(), dsnString, zlog, tracer)
	if err != nil {
		return fmt.Errorf("unable to setup sql sinker: %w", err)
	}

	app.SuperviseAndStart(sqlSinker)

	return app.WaitForTermination(zlog, 0*time.Second, 30*time.Second)
}

var clickhouseSetupCmd = sfcli.Command(clickhouseSetupE,
	"setup <dsn> <manifest>",
	"Setup the required infrastructure for a Substreams SQL deployable unit (ClickHouse)",
	sfcli.ExactArgs(2),
	sfcli.Flags(func(flags *pflag.FlagSet) {
		sqlAddCommonDatabaseChangesFlags(flags)
		sqlAddCommonSinkerFlags(flags)
		flags.Bool("system-tables-only", false, "Will only create/update the system tables (cursors, substreams_history) and ignore the schema from the manifest")
		flags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")
	}),
	sfcli.OnCommandErrorLogAndExit(zlog),
)

func clickhouseSetupE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	dsnString := args[0]
	manifestPath := args[1]

	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("setup manifest reader: %w", err)
	}
	pkgBundle, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	options := sinker2.SinkerSetupOptions{
		CursorTableName:            sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:           sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:          sflags.MustGetString(cmd, "clickhouse-cluster"),
		OnModuleHashMismatch:       sqlResolveModuleHashMismatchFlag(cmd),
		SystemTablesOnly:           sflags.MustGetBool(cmd, "system-tables-only"),
		IgnoreDuplicateTableErrors: sflags.MustGetBool(cmd, "ignore-duplicate-table-errors"),
	}

	return sinker2.SinkerSetup(ctx, dsnString, pkgBundle.Package, options, zlog, tracer)
}
