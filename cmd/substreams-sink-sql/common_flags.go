package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
)

var (
	onModuleHashMismatchFlag            = "on-module-hash-mismatch"  // new, correct spelling
	onModuleHashMistmatchFlagDeprecated = "on-module-hash-mistmatch" // old, typo (deprecated)
)

var supportedOutputTypes = "sf.substreams.sink.database.v1.DatabaseChanges,sf.substreams.database.v1.DatabaseChanges"

// AddCommonSinkerFlags adds the flags common to all command that needs to create a sinker,
// namely the `run` and `generate-csv` commands.
func AddCommonSinkerFlags(flags *pflag.FlagSet) {
	flags.String(onModuleHashMismatchFlag, "error", cli.FlagDescription(`
		What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'

		- If 'error' is used (default), it will exit with an error explaining the problem and how to fix it.
		- If 'warn' is used, it does the same as 'ignore' but it will log a warning message when it happens.
		- If 'ignore' is set, we pick the cursor at the highest block number and use it as the starting point. Subsequent
		updates to the cursor will overwrite the module hash in the database.
	`))
	// Register deprecated flag for backward compatibility
	flags.String(onModuleHashMistmatchFlagDeprecated, "error", "(deprecated) use --on-module-hash-mismatch instead")
	flags.Lookup(onModuleHashMistmatchFlagDeprecated).Deprecated = "use --on-module-hash-mismatch instead"
}

func AddCommonDatabaseChangesFlags(flags *pflag.FlagSet) {
	flags.String("cursors-table", "cursors", "[Operator] Name of the table to use for storing cursors")
	flags.String("history-table", "substreams_history", "[Operator] Name of the table to use for storing block history, used to handle reorgs")
	flags.String("clickhouse-cluster", "", "[Operator] If non-empty, a 'ON CLUSTER <cluster>' clause will be applied when setting up tables in Clickhouse. It will also replace the table engine with it's replicated counterpart (MergeTree will be replaced with ReplicatedMergeTree for example).")
	flags.String("bytes-encoding", "raw", "[Schema] Encoding for protobuf bytes fields: raw, hex, 0xhex, base64, base58. Non-raw encodings store data as string type in database.")
}

func readBlockRangeArgument(in string) (blockRange *bstream.Range, err error) {
	// This replaces the old sink.ReadBlockRange which was removed in the new sink API.
	// bstream.ParseRange handles the same block range format (e.g., "100:200", "100:", ":200")
	return bstream.ParseRange(in)
}

type cliApplication struct {
	appCtx  context.Context
	shutter *shutter.Shutter
}

func (a *cliApplication) WaitForTermination(logger *zap.Logger, unreadyPeriodAfterSignal, gracefulShutdownDelay time.Duration) error {
	// On any exit path, we synchronize the logger one last time
	defer func() {
		logger.Sync()
	}()

	signalHandler, isSignaled, _ := cli.SetupSignalHandler(unreadyPeriodAfterSignal, logger)
	select {
	case <-signalHandler:
		go a.shutter.Shutdown(nil)
		break
	case <-a.shutter.Terminating():
		logger.Info("run terminating", zap.Bool("from_signal", isSignaled.Load()), zap.Bool("with_error", a.shutter.Err() != nil))
		break
	}

	logger.Info("waiting for run termination")
	select {
	case <-a.shutter.Terminated():
	case <-time.After(gracefulShutdownDelay):
		logger.Warn("application did not terminate within graceful period of " + gracefulShutdownDelay.String() + ", forcing termination")
	}

	if err := a.shutter.Err(); err != nil {
		return err
	}

	logger.Info("run terminated gracefully")
	return nil
}

// resolveOnModuleHashMismatchFlag resolves the on-module-hash-mismatch flag with deprecation support.
func resolveOnModuleHashMismatchFlag(cmd *cobra.Command) string {
	// Use deprecated value if set by provider
	if value, provided := sflags.MustGetStringProvided(cmd, onModuleHashMistmatchFlagDeprecated); provided {
		return value
	}

	// Deprecated flag wasn't explicitly set, return default from correct flag
	return sflags.MustGetString(cmd, onModuleHashMismatchFlag)
}
