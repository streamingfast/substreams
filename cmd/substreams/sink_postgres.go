package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	dbchangessinker "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	"github.com/streamingfast/substreams/sink"
)

func init() {
	sink.AddFlagsToSet(sinkPostgresCmd.Flags(),
		sink.FlagExcludeDefault(sink.FlagUndoBufferSize),
	)

	sinkPostgresCmd.Flags().Int(sink.FlagUndoBufferSize, 0, "Number of blocks to keep buffered to handle fork reorganizations; set to 0 (default) to handle reorgs in the PostgreSQL database via the history table")
	sinkPostgresCmd.Flags().Int("batch-block-flush-interval", 1_000, "Flush every N blocks in catch-up mode (0 to disable); ineffective in live mode")
	sinkPostgresCmd.Flags().Int("batch-row-flush-interval", 100_000, "Flush every N rows in catch-up mode (0 to disable); ineffective in live mode")
	sinkPostgresCmd.Flags().Int("live-block-flush-interval", 1, "Flush every N blocks in live mode")
	sinkPostgresCmd.Flags().Int("flush-retry-count", 3, "Number of flush retry attempts before failing")
	sinkPostgresCmd.Flags().Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures")
	sinkPostgresCmd.Flags().String("on-module-hash-mismatch", "error", "Action when module hash mismatches the stored value: 'error', 'warn', or 'ignore'")
	sinkPostgresCmd.Flags().String("cursors-table", "cursors", "Table name for storing cursors")
	sinkPostgresCmd.Flags().String("history-table", "substreams_history", "Table name for storing block history used by reorg handling")

	SinkCmd.AddCommand(sinkPostgresCmd)
}

var sinkPostgresCmd = &cobra.Command{
	Use:   "postgres <dsn> [<manifest> [<module_name>]]",
	Short: "Sink a DatabaseChanges substreams to a PostgreSQL database",
	Long: `Sink a substreams module emitting sf.substreams.sink.database.v1.DatabaseChanges messages to a PostgreSQL database.

The DSN must be a PostgreSQL connection string, e.g.:
  postgres://user:password@localhost:5432/mydb?sslmode=disable

The manifest is optional; if omitted, the command looks for 'substreams.yaml' in the current directory.

Before running for the first time, set up the database schema with:
  substreams sink setup <dsn> <manifest>`,
	RunE:         sinkPostgresE,
	Args:         cobra.RangeArgs(1, 3),
	SilenceUsage: true,
}

const postgresOutputTypes = "sf.substreams.sink.database.v1.DatabaseChanges,sf.substreams.database.v1.DatabaseChanges"

func sinkPostgresE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	dsnString := args[0]
	if !strings.HasPrefix(dsnString, "postgres://") && !strings.HasPrefix(dsnString, "postgresql://") {
		return fmt.Errorf("DSN %q is not a valid PostgreSQL connection string (must start with 'postgres://' or 'postgresql://')", dsnString)
	}

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	baseSink, err := sink.NewFromViper(cmd, postgresOutputTypes, manifestPath, outputModule, "sink_postgres", zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating base sinker: %w", err)
	}

	dbchangessinker.RegisterMetrics()

	sinkerFactory := dbchangessinker.SinkerFactory(baseSink, dbchangessinker.SinkerFactoryOptions{
		CursorTableName:         sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:        sflags.MustGetString(cmd, "history-table"),
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
		return fmt.Errorf("creating postgres sinker: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	sqlSinker.Run(ctx)
	return sqlSinker.Err()
}
