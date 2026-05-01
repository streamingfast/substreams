package main

import (
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	sfcli "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	sinker2 "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

const sqlSupportedOutputTypes = "sf.substreams.sink.database.v1.DatabaseChanges,sf.substreams.database.v1.DatabaseChanges"

var (
	sqlOnModuleHashMismatchFlag            = "on-module-hash-mismatch"
	sqlOnModuleHashMistmatchFlagDeprecated = "on-module-hash-mistmatch"
)

func init() {
	postgresCmd.AddCommand(postgresRunCmd)
	postgresCmd.AddCommand(postgresSetupCmd)
	SinkCmd.AddCommand(postgresCmd)
}

var postgresCmd = &cobra.Command{
	Use:   "postgres",
	Short: "PostgreSQL sink commands",
}

var postgresRunCmd = sfcli.Command(postgresRunE,
	"run <dsn> <manifest> [<start>:<stop>]",
	"Runs PostgreSQL sink process",
	sfcli.RangeArgs(2, 3),
	sfcli.Flags(func(flags *pflag.FlagSet) {
		sink.AddFlagsToSet(flags, sink.FlagExcludeDefault("undo-buffer-size"))
		sqlAddCommonSinkerFlags(flags)
		sqlAddCommonDatabaseChangesFlags(flags)
		flags.Int("undo-buffer-size", 0, "If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database (required for some databases like Postgres).")
		flags.Int("batch-block-flush-interval", 1_000, "When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval.")
		flags.Int("batch-row-flush-interval", 100_000, "When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval.")
		flags.Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks.")
		flags.Int("flush-retry-count", 3, "Number of retry attempts for flush operations.")
		flags.Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures.")
	}),
	sfcli.OnCommandErrorLogAndExit(zlog),
)

func postgresRunE(cmd *cobra.Command, args []string) error {
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

var postgresSetupCmd = sfcli.Command(postgresSetupE,
	"setup <dsn> <manifest>",
	"Setup the required infrastructure for a Substreams SQL deployable unit (PostgreSQL)",
	sfcli.ExactArgs(2),
	sfcli.Flags(func(flags *pflag.FlagSet) {
		sqlAddCommonDatabaseChangesFlags(flags)
		sqlAddCommonSinkerFlags(flags)
		flags.Bool("postgraphile", false, "Will append the necessary 'comments' on cursors table to fully support postgraphile")
		flags.Bool("system-tables-only", false, "Will only create/update the system tables (cursors, substreams_history) and ignore the schema from the manifest")
		flags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")
	}),
	sfcli.OnCommandErrorLogAndExit(zlog),
)

func postgresSetupE(cmd *cobra.Command, args []string) error {
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
		Postgraphile:               sflags.MustGetBool(cmd, "postgraphile"),
	}

	return sinker2.SinkerSetup(ctx, dsnString, pkgBundle.Package, options, zlog, tracer)
}

func sqlAddCommonSinkerFlags(flags *pflag.FlagSet) {
	flags.String(sqlOnModuleHashMismatchFlag, "error", sfcli.FlagDescription(`
		What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'

		- If 'error' is used (default), it will exit with an error explaining the problem and how to fix it.
		- If 'warn' is used, it does the same as 'ignore' but it will log a warning message when it happens.
		- If 'ignore' is set, we pick the cursor at the highest block number and use it as the starting point. Subsequent
		updates to the cursor will overwrite the module hash in the database.
	`))
	flags.String(sqlOnModuleHashMistmatchFlagDeprecated, "error", "(deprecated) use --on-module-hash-mismatch instead")
	flags.Lookup(sqlOnModuleHashMistmatchFlagDeprecated).Deprecated = "use --on-module-hash-mismatch instead"
}

func sqlAddCommonDatabaseChangesFlags(flags *pflag.FlagSet) {
	flags.String("cursors-table", "cursors", "[Operator] Name of the table to use for storing cursors")
	flags.String("history-table", "substreams_history", "[Operator] Name of the table to use for storing block history, used to handle reorgs")
	flags.String("clickhouse-cluster", "", "[Operator] If non-empty, a 'ON CLUSTER <cluster>' clause will be applied when setting up tables in Clickhouse. It will also replace the table engine with its replicated counterpart (MergeTree will be replaced with ReplicatedMergeTree for example).")
	flags.String("bytes-encoding", "raw", "[Schema] Encoding for protobuf bytes fields: raw, hex, 0xhex, base64, base58. Non-raw encodings store data as string type in database.")
}

func sqlResolveModuleHashMismatchFlag(cmd *cobra.Command) string {
	if value, provided := sflags.MustGetStringProvided(cmd, sqlOnModuleHashMistmatchFlagDeprecated); provided {
		return value
	}
	return sflags.MustGetString(cmd, sqlOnModuleHashMismatchFlag)
}

func sqlSetBlockRangeFlags(cmd *cobra.Command, rangeArg string) error {
	parts := strings.SplitN(rangeArg, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected format <start>:<stop>")
	}

	startStr := strings.TrimSpace(parts[0])
	stopStr := strings.TrimSpace(parts[1])

	if startStr != "" {
		if err := cmd.Flags().Set("start-block", startStr); err != nil {
			return fmt.Errorf("setting start-block flag: %w", err)
		}
	}

	if stopStr != "" {
		if err := cmd.Flags().Set("stop-block", stopStr); err != nil {
			return fmt.Errorf("setting stop-block flag: %w", err)
		}
	}

	return nil
}
