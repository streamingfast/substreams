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
	sink.AddFlagsToSet(sinkClickhouseCmd.Flags())
	sinkClickhouseCmd.Flags().String("cursor-table-name", "cursors", "Table to use for storing cursors")
	sinkClickhouseCmd.Flags().String("clickhouse-cluster", "", "If non-empty, adds 'ON CLUSTER <cluster>' clause and uses replicated table engines")
	sinkClickhouseCmd.Flags().Int("batch-block-flush-interval", 1000, "Number of blocks between flushes in batch mode (0 to disable)")
	sinkClickhouseCmd.Flags().Int("batch-row-flush-interval", 100000, "Number of rows between flushes in batch mode (0 to disable)")
	sinkClickhouseCmd.Flags().Int("live-block-flush-interval", 1, "Number of blocks between flushes in live mode")
	sinkClickhouseCmd.Flags().String("on-module-hash-mismatch", "error", "What to do when module hash mismatch: error, warn, ignore")
	sinkClickhouseCmd.Flags().Int("flush-retry-count", 3, "Number of flush retry attempts on error")
	sinkClickhouseCmd.Flags().Duration("flush-retry-delay", 1*time.Second, "Base delay between flush retries")
	SinkCmd.AddCommand(sinkClickhouseCmd)
}

var sinkClickhouseCmd = &cobra.Command{
	Use:   "clickhouse <dsn> [<manifest> [<module_name>]]",
	Short: "Sink Substreams data to a ClickHouse database",
	RunE:  sinkClickhouseE,
	Args:  cobra.RangeArgs(1, 3),
}

func sinkClickhouseE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	dsn := args[0]

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args[1:])
	if err != nil {
		return err
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	baseSink, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink_clickhouse", zlog, tracer)
	if err != nil {
		return err
	}

	cursorTableName, _ := cmd.Flags().GetString("cursor-table-name")
	clickhouseCluster, _ := cmd.Flags().GetString("clickhouse-cluster")
	batchBlockFlushInterval, _ := cmd.Flags().GetInt("batch-block-flush-interval")
	batchRowFlushInterval, _ := cmd.Flags().GetInt("batch-row-flush-interval")
	liveBlockFlushInterval, _ := cmd.Flags().GetInt("live-block-flush-interval")
	onModuleHashMismatch, _ := cmd.Flags().GetString("on-module-hash-mismatch")
	flushRetryCount, _ := cmd.Flags().GetInt("flush-retry-count")
	flushRetryDelay, _ := cmd.Flags().GetDuration("flush-retry-delay")

	sinker.RegisterMetrics()

	factoryFunc := sinker.SinkerFactory(baseSink, sinker.SinkerFactoryOptions{
		CursorTableName:         cursorTableName,
		ClickhouseCluster:       clickhouseCluster,
		BatchBlockFlushInterval: batchBlockFlushInterval,
		BatchRowFlushInterval:   batchRowFlushInterval,
		LiveBlockFlushInterval:  liveBlockFlushInterval,
		OnModuleHashMismatch:    onModuleHashMismatch,
		HandleReorgs:            false, // ClickHouse only supports inserts, no reorg management
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
