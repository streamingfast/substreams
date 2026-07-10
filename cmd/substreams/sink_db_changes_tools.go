package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
)

var sinkDBChangesToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Tools for developers and operators",
}

var sinkDBChangesToolsCursorCmd = &cobra.Command{
	Use:   "cursor",
	Short: "Tools related to cursor handling (read/write)",
}

var sinkDBChangesToolsReadCursorCmd = &cobra.Command{
	Use:   "read",
	Short: "[Operator] Read active cursor(s) from database, if present",
	Long: cli.Dedent(`
		This command is going to fetch all known cursors from the database. In the database,
		a cursor is saved per module's hash which mean if you update your '.spkg', you might
		end up with multiple cursors for different module.

		This command will list all of them.
	`),
	RunE: toolsReadCursorE,
}

var sinkDBChangesToolsWriteCursorCmd = &cobra.Command{
	Use:   "write <module_hash> <cursor>",
	Short: "[Operator] Write a new active cursor for a given module's hash in the database",
	Long: cli.Dedent(`
		**Warning** This can screw up the sink's state, use only if you know what you are doing.

		This command is going to write a new cursor in the database for the given module's
		hash. The command update the current cursor if it exists or insert a new one if
		none already exist.
	`),
	Args: cobra.ExactArgs(2),
	RunE: toolsWriteCursorE,
}

var sinkDBChangesToolsDeleteCursorCmd = &cobra.Command{
	Use:   "delete [<module_hash>]",
	Short: "[Operator] Delete the active cursor for a given module's hash in the database",
	Long: cli.Dedent(`
		**Warning** This can screw up the sink's state, use only if you know what you are doing.

		This command is going to delete the cursor in the database for the given module's
		hash. If the cursor does not exist, the command assume it is correctly deleted.
	`),
	Args: cobra.RangeArgs(0, 1),
	RunE: toolsDeleteCursorE,
}

func init() {
	addToolsCursorFlags := func(flags *pflag.FlagSet) {
		flags.String("cursors-table", "cursors", "[Operator] Name of the table to use for storing cursors")
		flags.String("history-table", "substreams_history", "[Operator] Name of the table to use for storing block history, used to handle reorgs")
		flags.String("clickhouse-cluster", "", "[Operator] If non-empty, a 'ON CLUSTER <cluster>' clause will be applied when setting up tables in Clickhouse. It will also replace the table engine with it's replicated counterpart (MergeTree will be replaced with ReplicatedMergeTree for example).")
	}

	addToolsCursorFlags(sinkDBChangesToolsReadCursorCmd.Flags())
	addToolsCursorFlags(sinkDBChangesToolsWriteCursorCmd.Flags())
	addToolsCursorFlags(sinkDBChangesToolsDeleteCursorCmd.Flags())
	sinkDBChangesToolsDeleteCursorCmd.Flags().BoolP("all", "a", false, "Delete all active cursors")

	sinkDBChangesToolsCursorCmd.AddCommand(sinkDBChangesToolsReadCursorCmd)
	sinkDBChangesToolsCursorCmd.AddCommand(sinkDBChangesToolsWriteCursorCmd)
	sinkDBChangesToolsCursorCmd.AddCommand(sinkDBChangesToolsDeleteCursorCmd)
	sinkDBChangesToolsCmd.AddCommand(sinkDBChangesToolsCursorCmd)
	sinkDBChangesCmd.AddCommand(sinkDBChangesToolsCmd)
}

func toolsReadCursorE(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	sinkDBPreStart(cmd)

	loader, err := toolsCreateLoader(cmd, dsnString)
	if err != nil {
		return err
	}

	out, err := loader.GetAllCursors(cmd.Context())
	cli.NoError(err, "Unable to get all cursors")

	if len(out) == 0 {
		fmt.Println("No cursor(s) present in the database")
		return nil
	}

	for id, cursor := range out {
		fmt.Printf("Module %s: Block %s [%s]\n", id, cursor.Block(), cursorToShortString(cursor))
	}

	return nil
}

func toolsWriteCursorE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	moduleHash := args[0]
	opaqueCursor := args[1]

	cli.Ensure(moduleHash != "", "The <module_hash> cannot be empty")
	cli.Ensure(len(moduleHash) == 40, "The <module_hash> must be exactly 40 characters long")

	cursor, err := sink.NewCursor(opaqueCursor)
	cli.NoError(err, "The <cursor> is invalid")

	sinkDBPreStart(cmd)

	loader, err := toolsCreateLoader(cmd, dsnString)
	if err != nil {
		return err
	}

	err = loader.UpdateCursor(cmd.Context(), nil, moduleHash, cursor)
	if err != nil {
		if errors.Is(err, db.ErrCursorNotFound) {
			err = loader.InsertCursor(cmd.Context(), moduleHash, cursor)
			cli.NoError(err, "Unable to insert cursor")
		}

		cli.NoError(err, "Unable to update cursor")
	}

	fmt.Println("Cursor written successfully")
	fmt.Printf("- Block %s\n", cursor.Block())
	fmt.Printf("- Head Block %s\n", cursor.HeadBlock)
	fmt.Printf("- LIB Block %s\n", cursor.LIB)
	fmt.Printf("- Step %q\n", cursor.Step)
	fmt.Printf("- Cursor %q\n", cursorToShortString(cursor))
	return nil
}

func toolsDeleteCursorE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	moduleHash := ""
	if !sflags.MustGetBool(cmd, "all") {
		cli.Ensure(len(args) == 1, "Module hash is required, if you want to delete all cursors, use --all to avoid specifying a module's hash")

		moduleHash = args[0]

		cli.Ensure(moduleHash != "", "The <module_hash> cannot be empty")
		cli.Ensure(len(moduleHash) == 40, "The <module_hash> must be exactly 40 characters long")
	}

	sinkDBPreStart(cmd)

	loader, err := toolsCreateLoader(cmd, dsnString)
	if err != nil {
		return err
	}

	if moduleHash == "" {
		deletedCount, err := loader.DeleteAllCursors(cmd.Context())
		cli.NoError(err, "Unable to delete cursor")

		fmt.Printf("Deleted %d cursor(s) successfully\n", deletedCount)
	} else {
		err := loader.DeleteCursor(cmd.Context(), moduleHash)
		if err != nil && !errors.Is(err, db.ErrCursorNotFound) {
			cli.NoError(err, "Unable to delete cursor")
		}

		fmt.Println("Cursor delete successfully")
	}

	return nil
}

func toolsCreateLoader(cmd *cobra.Command, dsnString string) (*db.Loader, error) {
	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	handleReorgs := false
	loader, err := db.NewLoader(
		dsn,
		cursorTableName,
		historyTableName,
		sflags.MustGetString(cmd, "clickhouse-cluster"),
		0, 0, 0,
		db.OnModuleHashMismatchError.String(),
		&handleReorgs,
		zlog, tracer,
	)
	if err != nil {
		return nil, fmt.Errorf("creating loader: %w", err)
	}

	if err := loader.LoadTables(dsn.Schema(), cursorTableName, historyTableName); err != nil {
		var systemTableError *db.SystemTableError
		if errors.As(err, &systemTableError) {
			fmt.Printf("Error validating the system table: %s\n", systemTableError)
			fmt.Println("Did you run setup ?")
			os.Exit(1)
		}

		cli.NoError(err, "Unable to load table information from database")
	}

	return loader, nil
}

func cursorToShortString(in *sink.Cursor) string {
	cursor := in.String()
	if len(cursor) > 12 {
		cursor = cursor[0:6] + "..." + cursor[len(cursor)-6:]
	}

	return cursor
}
