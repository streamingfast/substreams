package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	. "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	sinker2 "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	sink "github.com/streamingfast/substreams/sink"
)

var sinkRunCmd = Command(sinkRunE,
	"run <dsn> <manifest> [<start>:<stop>]",
	"Runs SQL sink process",
	RangeArgs(2, 3),
	Flags(func(flags *pflag.FlagSet) {
		sink.AddFlagsToSet(flags, sink.FlagExcludeDefault("undo-buffer-size"))
		AddCommonSinkerFlags(flags)
		AddCommonDatabaseChangesFlags(flags)

		flags.Int("undo-buffer-size", 0, "If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database (required for some databases like Postgres).")
		flags.Int("batch-block-flush-interval", 1_000, "When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
		flags.Int("batch-row-flush-interval", 100_000, "When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
		flags.Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks.")
		flags.Int("flush-interval", 0, "(deprecated) please use --batch-block-flush-interval instead")
		flags.Int("flush-retry-count", 3, "Number of retry attempts for flush operations")
		flags.Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures")
	}),
	Example("substreams-sink-sql run 'postgres://localhost:5432/posgres?sslmode=disable' uniswap-v3@v0.2.10"),
	OnCommandErrorLogAndExit(zlog),
)

func sinkRunE(cmd *cobra.Command, args []string) error {
	app := NewApplication(cmd.Context())

	sinker2.RegisterMetrics()

	dsnString := args[0]
	manifestPath := args[1]

	// Handle third argument - can be either block range or module name
	// For backward compatibility, if it contains ':', treat it as block range
	if len(args) > 2 {
		thirdArg := args[2]
		// Check if it looks like a block range (contains ':')
		if strings.Contains(thirdArg, ":") {
			// Parse and set block range flags to bridge with substreams/sink library
			br, err := readBlockRangeArgument(thirdArg)
			if err != nil {
				return fmt.Errorf("invalid block range %q: %w", thirdArg, err)
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
		} else {
			// Treat as module name (new behavior, for forward compatibility)
			// Module name is handled via sink.InferOutputModuleFromPackage by default
			// or can be overridden, but we'll just pass empty string to let it infer
		}
	}

	sk, err := sink.NewFromViper(
		cmd,
		supportedOutputTypes,
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
	if sflags.MustGetInt(cmd, "flush-interval") != 0 {
		batchBlockFlushInterval = sflags.MustGetInt(cmd, "flush-interval")
	}
	batchRowFlushInterval := sflags.MustGetInt(cmd, "batch-row-flush-interval")
	liveBlockFlushInterval := sflags.MustGetInt(cmd, "live-block-flush-interval")
	flushRetryCount := sflags.MustGetInt(cmd, "flush-retry-count")
	flushRetryDelay := sflags.MustGetDuration(cmd, "flush-retry-delay")

	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")
	handleReorgs := sflags.MustGetInt(cmd, "undo-buffer-size") == 0

	sinkerFactory := sinker2.SinkerFactory(sk, sinker2.SinkerFactoryOptions{
		CursorTableName:         cursorTableName,
		HistoryTableName:        historyTableName,
		ClickhouseCluster:       sflags.MustGetString(cmd, "clickhouse-cluster"),
		BatchBlockFlushInterval: batchBlockFlushInterval,
		BatchRowFlushInterval:   batchRowFlushInterval,
		LiveBlockFlushInterval:  liveBlockFlushInterval,
		OnModuleHashMismatch:    resolveOnModuleHashMismatchFlag(cmd),
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
