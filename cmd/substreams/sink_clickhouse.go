package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
)

const sinkClickhouseDriver = "clickhouse"

var sinkClickhouseCmd = &cobra.Command{
	Use:     "clickhouse [<manifest> [<module>]]",
	Short:   "Sink Substreams data into a ClickHouse database",
	Long:    "Sink Substreams data into a ClickHouse database, auto-detecting the mode from the output module type.",
	Example: "substreams sink clickhouse uniswap-v3@v0.2.10 --dsn 'clickhouse://localhost:9000/default'",
	Args:    sinkEngineParentArgs,
	RunE:    newSinkRunE(sinkClickhouseDriver),
}

var sinkClickhouseSetupCmd = &cobra.Command{
	Use:   "setup <manifest> [<module>]",
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
	Args: cobra.RangeArgs(1, 2),
	RunE: newSinkSetupE(sinkClickhouseDriver),
}

func init() {
	persistent := sinkClickhouseCmd.PersistentFlags()
	addDSNFlag(persistent)
	addOperatorFlags(persistent)

	addSinkRunFlags(sinkClickhouseCmd.Flags(), sinkClickhouseDriver)
	setModeGroupedUsage(sinkClickhouseCmd)

	setupFlags := sinkClickhouseSetupCmd.Flags()
	addCursorTableFlags(setupFlags)
	addClusterFlag(setupFlags)
	addOnModuleHashMismatchFlag(setupFlags)
	setupFlags.Bool("system-tables-only", false, "will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
	setupFlags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")
	addBytesEncodingFlag(setupFlags)
	addFromProtoSchemaFlags(setupFlags)
	addClickhouseStateFlags(setupFlags)

	sinkClickhouseCmd.AddCommand(sinkClickhouseSetupCmd)
	sinkClickhouseCmd.AddCommand(newSinkToolsCmd(sinkClickhouseDriver))

	SinkCmd.AddCommand(sinkClickhouseCmd)
}
