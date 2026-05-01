package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	dbchangessinker "github.com/streamingfast/substreams-sink-sql/db_changes/sinker"
	"github.com/streamingfast/substreams/sink"
)

func init() {
	sink.AddFlagsToSet(sinkClickhouseCmd.Flags(),
		sink.FlagExcludeDefault(sink.FlagUndoBufferSize),
	)

	sinkClickhouseCmd.Flags().Int(sink.FlagUndoBufferSize, 12, "Number of blocks to keep buffered to handle fork reorganizations; a non-zero buffer is recommended for ClickHouse to avoid reorg handling in the database")
	sinkClickhouseCmd.Flags().String("clickhouse-cluster", "", "ClickHouse cluster name; if non-empty, applies 'ON CLUSTER <cluster>' when creating tables and uses replicated table engines")
	sinkClickhouseCmd.Flags().Int("batch-block-flush-interval", 1_000, "Flush every N blocks in catch-up mode (0 to disable); ineffective in live mode")
	sinkClickhouseCmd.Flags().Int("batch-row-flush-interval", 100_000, "Flush every N rows in catch-up mode (0 to disable); ineffective in live mode")
	sinkClickhouseCmd.Flags().Int("live-block-flush-interval", 1, "Flush every N blocks in live mode")
	sinkClickhouseCmd.Flags().Int("flush-retry-count", 3, "Number of flush retry attempts before failing")
	sinkClickhouseCmd.Flags().Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures")
	sinkClickhouseCmd.Flags().String("on-module-hash-mismatch", "error", "Action when module hash mismatches the stored value: 'error', 'warn', or 'ignore'")
	sinkClickhouseCmd.Flags().String("cursors-table", "cursors", "Table name for storing cursors")
	sinkClickhouseCmd.Flags().String("history-table", "substreams_history", "Table name for storing block history used by reorg handling")

	SinkCmd.AddCommand(sinkClickhouseCmd)
}

var sinkClickhouseCmd = &cobra.Command{
	Use:   "clickhouse <dsn> [<manifest> [<module_name>]]",
	Short: "Sink a DatabaseChanges substreams to a ClickHouse database",
	Long: `Sink a substreams module emitting sf.substreams.sink.database.v1.DatabaseChanges messages to a ClickHouse database.

The DSN must be a ClickHouse connection string, e.g.:
  clickhouse://user:password@localhost:9000/mydb

The manifest is optional; if omitted, the command looks for 'substreams.yaml' in the current directory.

Before running for the first time, set up the database schema with:
  substreams-sink-sql setup <dsn> <manifest>`,
	RunE:         sinkClickhouseE,
	Args:         cobra.RangeArgs(1, 3),
	SilenceUsage: true,
}

const clickhouseOutputTypes = "sf.substreams.sink.database.v1.DatabaseChanges,sf.substreams.database.v1.DatabaseChanges"

func sinkClickhouseE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	dsnString := args[0]
	if !strings.HasPrefix(dsnString, "clickhouse://") {
		return fmt.Errorf("DSN %q is not a valid ClickHouse connection string (must start with 'clickhouse://')", dsnString)
	}

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	baseSink, err := sink.NewFromViper(cmd, clickhouseOutputTypes, manifestPath, outputModule, "sink_clickhouse", zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating base sinker: %w", err)
	}

	dbchangessinker.RegisterMetrics()

	sinkerFactory := dbchangessinker.SinkerFactory(baseSink, dbchangessinker.SinkerFactoryOptions{
		CursorTableName:         sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:        sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:       sflags.MustGetString(cmd, "clickhouse-cluster"),
		BatchBlockFlushInterval: sflags.MustGetInt(cmd, "batch-block-flush-interval"),
		BatchRowFlushInterval:   sflags.MustGetInt(cmd, "batch-row-flush-interval"),
		LiveBlockFlushInterval:  sflags.MustGetInt(cmd, "live-block-flush-interval"),
		OnModuleHashMismatch:    sflags.MustGetString(cmd, "on-module-hash-mismatch"),
		HandleReorgs:            baseSink.UndoBufferSize == 0,
		FlushRetryCount:         sflags.MustGetInt(cmd, "flush-retry-count"),
		FlushRetryDelay:         sflags.MustGetDuration(cmd, "flush-retry-delay"),
	})

	sqlSinker, err := sinkerFactory(ctx, dsnString, zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating clickhouse sinker: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	sqlSinker.Run(ctx)
	return sqlSinker.Err()
}
