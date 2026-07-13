package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/sink"
)

const sinkPostgresDriver = "postgres"

var sinkPostgresCmd = &cobra.Command{
	Use:   "postgres",
	Short: "Sink Substreams data into a PostgreSQL database",
}

var sinkPostgresRunCmd = &cobra.Command{
	Use:     "run [<manifest> [<module>]]",
	Short:   "Runs the PostgreSQL sink process, auto-detecting the mode from the output module type",
	Example: "substreams sink postgres run uniswap-v3@v0.2.10 --dsn 'postgres://localhost:5432/postgres?sslmode=disable'",
	Args:    cobra.RangeArgs(0, 2),
	RunE:    newSinkRunE(sinkPostgresDriver),
}

var sinkPostgresSetupCmd = &cobra.Command{
	Use:   "setup <manifest>",
	Short: "Setup the required infrastructure to deploy a Substreams SQL deployable unit",
	Args:  cobra.ExactArgs(1),
	RunE:  newSinkSetupE(sinkPostgresDriver),
}

var sinkPostgresCreateUserCmd = &cobra.Command{
	Use:   "create-user <username> <database>",
	Short: "Create a user in the database",
	Args:  cobra.ExactArgs(2),
	RunE:  newSinkCreateUserE(sinkPostgresDriver),
}

func init() {
	persistent := sinkPostgresCmd.PersistentFlags()
	addDSNFlag(persistent)
	addOperatorFlags(persistent)

	runFlags := sinkPostgresRunCmd.Flags()
	sink.AddFlagsToSet(runFlags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addBytesEncodingFlag(runFlags)
	addDatabaseChangesModeRunFlags(runFlags)
	addFromProtoModeRunFlags(runFlags, sinkPostgresDriver)

	setupFlags := sinkPostgresSetupCmd.Flags()
	addCursorTableFlags(setupFlags)
	addOnModuleHashMismatchFlag(setupFlags)
	setupFlags.Bool("postgraphile", false, "Will append the necessary 'comments' on cursors table to fully support postgraphile")
	setupFlags.Bool("system-tables-only", false, "will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
	setupFlags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")

	createUserFlags := sinkPostgresCreateUserCmd.Flags()
	addCursorTableFlags(createUserFlags)
	createUserFlags.Int("retries", 3, "Number of retries to attempt when a connection error occurs")
	createUserFlags.Bool("read-only", false, "Create a read-only user")
	createUserFlags.String("password-env", "", "Name of the environment variable containing the password")

	sinkPostgresCmd.AddCommand(sinkPostgresRunCmd)
	sinkPostgresCmd.AddCommand(sinkPostgresSetupCmd)
	sinkPostgresCmd.AddCommand(sinkPostgresCreateUserCmd)
	sinkPostgresCmd.AddCommand(newSinkToolsCmd(sinkPostgresDriver))

	SinkCmd.AddCommand(sinkPostgresCmd)
}
