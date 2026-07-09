package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/logging/zapx"
	"go.uber.org/zap"
)

var supportedOutputTypes = "sf.substreams.sink.database.v1.DatabaseChanges,sf.substreams.database.v1.DatabaseChanges"

var (
	onModuleHashMismatchFlag            = "on-module-hash-mismatch"
	onModuleHashMistmatchFlagDeprecated = "on-module-hash-mistmatch"
)

var sinkSQLCmd = &cobra.Command{
	Use:   "sql",
	Short: "Sink Substreams data into an SQL database (PostgreSQL, ClickHouse)",
}

func init() {
	flags := sinkSQLCmd.PersistentFlags()
	flags.String("dsn", "", "Database connection string (falls back to the SUBSTREAMS_SINK_SQL_DSN environment variable), e.g. \"psql://user:pass@localhost:5432/db?sslmode=disable\" or \"clickhouse://user:pass@localhost:9000/db\"")
	flags.Duration("delay-before-start", 0, "[Operator] Amount of time to wait before starting any internal processes, can be used to perform maintenance on the pod before actually letting it start")
	flags.String("metrics-listen-addr", "", "[Operator] If non-empty, the process will listen on this address for Prometheus metrics request(s)")
	flags.String("pprof-listen-addr", "", "[Operator] If non-empty, the process will listen on this address for pprof analysis (see https://golang.org/pkg/net/http/pprof/)")

	SinkCmd.AddCommand(sinkSQLCmd)
}

func resolveSinkSQLDSN(cmd *cobra.Command) (string, error) {
	dsn := sflags.MustGetString(cmd, "dsn")
	if dsn == "" {
		dsn = os.Getenv("SUBSTREAMS_SINK_SQL_DSN")
	}
	if dsn == "" {
		return "", fmt.Errorf("no DSN provided, use --dsn or set SUBSTREAMS_SINK_SQL_DSN (e.g. \"psql://user:pass@localhost:5432/db?sslmode=disable\" or \"clickhouse://user:pass@localhost:9000/db\")")
	}
	return dsn, nil
}

func sinkSQLPreStart(cmd *cobra.Command) {
	if delay := sflags.MustGetDuration(cmd, "delay-before-start"); delay > 0 {
		zlog.Info("sleeping to respect delay before start setting", zapx.HumanDuration("delay", delay))
		time.Sleep(delay)
	}

	if addr := sflags.MustGetString(cmd, "metrics-listen-addr"); addr != "" {
		zlog.Debug("starting prometheus metrics server", zap.String("listen_addr", addr))
		go dmetrics.Serve(addr)
	}

	if addr := sflags.MustGetString(cmd, "pprof-listen-addr"); addr != "" {
		go func() {
			zlog.Debug("starting pprof server", zap.String("listen_addr", addr))
			if err := http.ListenAndServe(addr, nil); err != nil {
				zlog.Warn("unable to start profiling server", zap.Error(err), zap.String("listen_addr", addr))
			}
		}()
	}
}

func addOperatorFlags(flags *pflag.FlagSet) {
	flags.Duration("delay-before-start", 0, "[Operator] Amount of time to wait before starting any internal processes, can be used to perform maintenance on the pod before actually letting it start")
	flags.String("metrics-listen-addr", "", "[Operator] If non-empty, the process will listen on this address for Prometheus metrics request(s)")
	flags.String("pprof-listen-addr", "", "[Operator] If non-empty, the process will listen on this address for pprof analysis (see https://golang.org/pkg/net/http/pprof/)")
}

func addCommonSinkerFlags(flags *pflag.FlagSet) {
	flags.String(onModuleHashMismatchFlag, "error", cli.FlagDescription(`
		What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'

		- If 'error' is used (default), it will exit with an error explaining the problem and how to fix it.
		- If 'warn' is used, it does the same as 'ignore' but it will log a warning message when it happens.
		- If 'ignore' is set, we pick the cursor at the highest block number and use it as the starting point. Subsequent
		updates to the cursor will overwrite the module hash in the database.
	`))
	flags.String(onModuleHashMistmatchFlagDeprecated, "error", "(deprecated) use --on-module-hash-mismatch instead")
	flags.Lookup(onModuleHashMistmatchFlagDeprecated).Deprecated = "use --on-module-hash-mismatch instead"
}

func addCommonDatabaseChangesFlags(flags *pflag.FlagSet) {
	flags.String("cursors-table", "cursors", "[Operator] Name of the table to use for storing cursors")
	flags.String("history-table", "substreams_history", "[Operator] Name of the table to use for storing block history, used to handle reorgs")
	flags.String("clickhouse-cluster", "", "[Operator] If non-empty, a 'ON CLUSTER <cluster>' clause will be applied when setting up tables in Clickhouse. It will also replace the table engine with it's replicated counterpart (MergeTree will be replaced with ReplicatedMergeTree for example).")
	flags.String("bytes-encoding", "raw", "[Schema] Encoding for protobuf bytes fields: raw, hex, 0xhex, base64, base58. Non-raw encodings store data as string type in database.")
}

func readBlockRangeArgument(in string) (*bstream.Range, error) {
	return bstream.ParseRange(in)
}

func resolveOnModuleHashMismatchFlag(cmd *cobra.Command) string {
	if value, provided := sflags.MustGetStringProvided(cmd, onModuleHashMistmatchFlagDeprecated); provided {
		return value
	}
	return sflags.MustGetString(cmd, onModuleHashMismatchFlag)
}
