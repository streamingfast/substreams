package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	. "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	sinker2 "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	"github.com/streamingfast/substreams/manifest"
)

var sinkSetupCmd = Command(sinkSetupE,
	"setup <dsn> <manifest>",
	"Setup the required infrastructure to deploy a Substreams SQL deployable unit",
	ExactArgs(2),
	Flags(func(flags *pflag.FlagSet) {
		AddCommonDatabaseChangesFlags(flags)
		AddCommonSinkerFlags(flags)

		flags.Bool("postgraphile", false, "Will append the necessary 'comments' on cursors table to fully support postgraphile")
		flags.Bool("system-tables-only", false, "will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
		flags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schemal.sql' file will not have run fully!")
	}),
)

func sinkSetupE(cmd *cobra.Command, args []string) error {
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
		OnModuleHashMismatch:       resolveOnModuleHashMismatchFlag(cmd),
		SystemTablesOnly:           sflags.MustGetBool(cmd, "system-tables-only"),
		IgnoreDuplicateTableErrors: sflags.MustGetBool(cmd, "ignore-duplicate-table-errors"),
		Postgraphile:               sflags.MustGetBool(cmd, "postgraphile"),
	}

	return sinker2.SinkerSetup(ctx, dsnString, pkgBundle.Package, options, zlog, tracer)
}
