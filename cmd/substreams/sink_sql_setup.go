package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

var sinkSQLSetupCmd = &cobra.Command{
	Use:   "setup <manifest>",
	Short: "Setup the required infrastructure to deploy a Substreams SQL deployable unit",
	Args:  cobra.ExactArgs(1),
	RunE:  sinkSQLSetupE,
}

func init() {
	flags := sinkSQLSetupCmd.Flags()
	addCommonDatabaseChangesFlags(flags)
	addCommonSinkerFlags(flags)

	flags.Bool("postgraphile", false, "Will append the necessary 'comments' on cursors table to fully support postgraphile")
	flags.Bool("system-tables-only", false, "will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
	flags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")

	sinkSQLCmd.AddCommand(sinkSQLSetupCmd)
}

func sinkSQLSetupE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()

	dsnString, err := resolveSinkSQLDSN(cmd)
	if err != nil {
		return err
	}

	manifestPath := args[0]

	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("setup manifest reader: %w", err)
	}
	pkgBundle, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	options := sinker.SinkerSetupOptions{
		CursorTableName:            sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:           sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:          sflags.MustGetString(cmd, "clickhouse-cluster"),
		OnModuleHashMismatch:       resolveOnModuleHashMismatchFlag(cmd),
		SystemTablesOnly:           sflags.MustGetBool(cmd, "system-tables-only"),
		IgnoreDuplicateTableErrors: sflags.MustGetBool(cmd, "ignore-duplicate-table-errors"),
		Postgraphile:               sflags.MustGetBool(cmd, "postgraphile"),
	}

	return sinker.SinkerSetup(ctx, dsnString, pkgBundle.Package, options, zlog, tracer)
}
