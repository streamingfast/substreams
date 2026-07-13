package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/sink"
)

const sinkClickhouseDriver = "clickhouse"

var sinkClickhouseCmd = &cobra.Command{
	Use:   "clickhouse",
	Short: "Sink Substreams data into a ClickHouse database",
}

var sinkClickhouseRunCmd = &cobra.Command{
	Use:     "run [<manifest> [<module>]]",
	Short:   "Runs the ClickHouse sink process, auto-detecting the mode from the output module type",
	Example: "substreams sink clickhouse run uniswap-v3@v0.2.10 --dsn 'clickhouse://localhost:9000/default'",
	Args:    cobra.RangeArgs(0, 2),
	RunE:    newSinkRunE(sinkClickhouseDriver),
}

var sinkClickhouseSetupCmd = &cobra.Command{
	Use:   "setup <manifest>",
	Short: "Setup the required infrastructure to deploy a Substreams SQL deployable unit",
	Args:  cobra.ExactArgs(1),
	RunE:  newSinkSetupE(sinkClickhouseDriver),
}

var sinkClickhouseCreateUserCmd = &cobra.Command{
	Use:   "create-user <username> <database>",
	Short: "Create a user in the database",
	Args:  cobra.ExactArgs(2),
	RunE:  newSinkCreateUserE(sinkClickhouseDriver),
}

func init() {
	persistent := sinkClickhouseCmd.PersistentFlags()
	addDSNFlag(persistent)
	addOperatorFlags(persistent)

	runFlags := sinkClickhouseRunCmd.Flags()
	sink.AddFlagsToSet(runFlags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addBytesEncodingFlag(runFlags)
	addClusterFlag(runFlags)
	addDatabaseChangesModeRunFlags(runFlags)
	addFromProtoModeRunFlags(runFlags, sinkClickhouseDriver)

	setupFlags := sinkClickhouseSetupCmd.Flags()
	addCursorTableFlags(setupFlags)
	addClusterFlag(setupFlags)
	addOnModuleHashMismatchFlag(setupFlags)
	setupFlags.Bool("system-tables-only", false, "will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
	setupFlags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")

	createUserFlags := sinkClickhouseCreateUserCmd.Flags()
	addCursorTableFlags(createUserFlags)
	addClusterFlag(createUserFlags)
	createUserFlags.Int("retries", 3, "Number of retries to attempt when a connection error occurs")
	createUserFlags.Bool("read-only", false, "Create a read-only user")
	createUserFlags.String("password-env", "", "Name of the environment variable containing the password")

	sinkClickhouseCmd.AddCommand(sinkClickhouseRunCmd)
	sinkClickhouseCmd.AddCommand(sinkClickhouseSetupCmd)
	sinkClickhouseCmd.AddCommand(sinkClickhouseCreateUserCmd)
	sinkClickhouseCmd.AddCommand(newSinkToolsCmd(sinkClickhouseDriver))

	SinkCmd.AddCommand(sinkClickhouseCmd)
}
