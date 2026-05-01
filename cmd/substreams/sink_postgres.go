package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/derr"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

func init() {
	sink.AddFlagsToSet(sinkPostgresCmd.Flags())
	sinkPostgresCmd.Flags().String("cursor-table-name", "cursors", "Table to use for storing cursors")
	sinkPostgresCmd.Flags().String("history-table-name", "substreams_history", "Table to use for storing block history for reorg support")
	sinkPostgresCmd.Flags().Int("batch-block-flush-interval", 1000, "Number of blocks between flushes in batch mode (0 to disable)")
	sinkPostgresCmd.Flags().Int("batch-row-flush-interval", 100000, "Number of rows between flushes in batch mode (0 to disable)")
	sinkPostgresCmd.Flags().Int("live-block-flush-interval", 1, "Number of blocks between flushes in live mode")
	sinkPostgresCmd.Flags().String("on-module-hash-mismatch", "error", "What to do when module hash mismatch: error, warn, ignore")
	sinkPostgresCmd.Flags().Bool("handle-reorgs", true, "Whether to handle reorgs in the database via a history table")
	sinkPostgresCmd.Flags().Int("flush-retry-count", 3, "Number of flush retry attempts on error")
	sinkPostgresCmd.Flags().Duration("flush-retry-delay", 1*time.Second, "Base delay between flush retries")
	SinkCmd.AddCommand(sinkPostgresCmd)
}

var sinkPostgresCmd = &cobra.Command{
	Use:   "postgres <dsn> [<manifest> [<module_name>]]",
	Short: "Sink Substreams data to a PostgreSQL database",
	RunE:  sinkPostgresE,
	Args:  cobra.RangeArgs(1, 3),
}

func sinkPostgresE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	dsn := args[0]

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	baseSink, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink_postgres", zlog, tracer)
	if err != nil {
		return err
	}

	cursorTableName, _ := cmd.Flags().GetString("cursor-table-name")
	historyTableName, _ := cmd.Flags().GetString("history-table-name")
	batchBlockFlushInterval, _ := cmd.Flags().GetInt("batch-block-flush-interval")
	batchRowFlushInterval, _ := cmd.Flags().GetInt("batch-row-flush-interval")
	liveBlockFlushInterval, _ := cmd.Flags().GetInt("live-block-flush-interval")
	onModuleHashMismatch, _ := cmd.Flags().GetString("on-module-hash-mismatch")
	handleReorgs, _ := cmd.Flags().GetBool("handle-reorgs")
	flushRetryCount, _ := cmd.Flags().GetInt("flush-retry-count")
	flushRetryDelay, _ := cmd.Flags().GetDuration("flush-retry-delay")

	sinker.RegisterMetrics()

	factoryFunc := sinker.SinkerFactory(baseSink, sinker.SinkerFactoryOptions{
		CursorTableName:         cursorTableName,
		HistoryTableName:        historyTableName,
		BatchBlockFlushInterval: batchBlockFlushInterval,
		BatchRowFlushInterval:   batchRowFlushInterval,
		LiveBlockFlushInterval:  liveBlockFlushInterval,
		OnModuleHashMismatch:    onModuleHashMismatch,
		HandleReorgs:            handleReorgs,
		FlushRetryCount:         flushRetryCount,
		FlushRetryDelay:         flushRetryDelay,
	})

	sqlSinker, err := factoryFunc(ctx, dsn, zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sql sinker: %w", err)
	}
	defer sqlSinker.Close()

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	sqlSinker.Run(ctx)
	return sqlSinker.Err()
}
