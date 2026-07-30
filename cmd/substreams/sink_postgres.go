package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
)

const sinkPostgresDriver = "postgres"

var sinkPostgresCmd = &cobra.Command{
	Use:     "postgres [<manifest> [<module>]]",
	Short:   "Sink Substreams data into a PostgreSQL database",
	Long:    "Sink Substreams data into a PostgreSQL database, auto-detecting the mode from the output module type.",
	Example: "substreams sink postgres uniswap-v3@v0.2.10 --dsn 'postgres://localhost:5432/postgres?sslmode=disable'",
	Args:    sinkEngineParentArgs,
	RunE:    newSinkRunE(sinkPostgresDriver),
}

var sinkPostgresSetupCmd = &cobra.Command{
	Use:   "setup <manifest>",
	Short: "Setup the required infrastructure to deploy a Substreams SQL deployable unit",
	Long: cli.Dedent(`
		Setup the database for the Substreams SQL sink, auto-detecting the mode from the
		output module type, exactly like the run action:

		- DatabaseChanges output: creates the system tables (cursors, history) and applies
		  the 'schema.sql' bundled in the manifest sink config.
		- Any other output type (from-proto): resolves the schema from the module's output
		  proto and creates the database schema and tables, then exits. This step is
		  idempotent and can be run again safely.
	`),
	Args: cobra.ExactArgs(1),
	RunE: newSinkSetupE(sinkPostgresDriver),
}

func init() {
	persistent := sinkPostgresCmd.PersistentFlags()
	addDSNFlag(persistent)
	addOperatorFlags(persistent)

	addSinkRunFlags(sinkPostgresCmd.Flags(), sinkPostgresDriver)

	setupFlags := sinkPostgresSetupCmd.Flags()
	addCursorTableFlags(setupFlags)
	addOnModuleHashMismatchFlag(setupFlags)
	setupFlags.Bool("postgraphile", false, "[DatabaseChanges mode] Will append the necessary 'comments' on cursors table to fully support postgraphile")
	setupFlags.Bool("system-tables-only", false, "[DatabaseChanges mode] will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
	setupFlags.Bool("ignore-duplicate-table-errors", false, "[DatabaseChanges mode][Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")
	addBytesEncodingFlag(setupFlags)
	addFromProtoModeRunFlags(setupFlags, sinkPostgresDriver)

	sinkPostgresCmd.AddCommand(sinkPostgresSetupCmd)
	sinkPostgresCmd.AddCommand(newSinkToolsCmd(sinkPostgresDriver))

	SinkCmd.AddCommand(sinkPostgresCmd)
}
