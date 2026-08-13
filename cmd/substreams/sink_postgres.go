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
	RunE: newSinkSetupE(sinkPostgresDriver),
}

var sinkPostgresConstraintsCmd = &cobra.Command{
	Use:   "constraints",
	Short: "Create or drop the schema's constraints on an already loaded database",
}

var sinkPostgresConstraintsApplyCmd = &cobra.Command{
	Use:   "apply <manifest> [<module>]",
	Short: "Create the schema's constraints on an already loaded database",
	Long: cli.Dedent(`
		Create the primary keys, unique and foreign key constraints of a from-proto schema
		on a database the sink has already loaded, skipping the ones already in place.

		The sink loads without them on purpose: measured through binary COPY, loading with
		foreign keys in place runs 27x slower than loading without, where building the very
		same constraints afterwards costs 3.3x. Creating them is a stop-the-world
		operation, though — every index is built and every foreign key validated, with the
		tables locked while it runs — so on a large database this belongs in a maintenance
		window, which is what --apply-constraints=manual leaves it to this command for.

		The index on _block_number_ is not created here. The sink creates that one when it
		starts, concurrently: this command is yours to schedule, and the reorg path cannot
		wait for a maintenance window.

		Running it again is safe: constraints already in place are left alone.

		The module is inferred from the package when it is left out. A package with more
		than one candidate has to be told which, or the schema this derives will not be the
		one the run created.
	`),
	Args: cobra.RangeArgs(1, 2),
	RunE: newSinkConstraintsE(sinkPostgresDriver, constraintsApply),
}

var sinkPostgresConstraintsDropCmd = &cobra.Command{
	Use:   "drop <manifest> [<module>]",
	Short: "Drop the schema's constraints",
	Long: cli.Dedent(`
		Drop the primary keys, unique and foreign key constraints of a from-proto schema,
		leaving anything the sink did not create alone — the index on _block_number_
		included, that one being the sink's own and recreated when it next starts.

		This is the escape hatch after --apply-constraints=always, and what makes a
		backfill that has to be resumed fast again without setting the schema up afresh:
		loading with foreign keys in place measured 27x slower than loading without them.

		Running it again is safe: anything already absent is skipped.
	`),
	Args: cobra.RangeArgs(1, 2),
	RunE: newSinkConstraintsE(sinkPostgresDriver, constraintsDrop),
}

func init() {
	persistent := sinkPostgresCmd.PersistentFlags()
	addDSNFlag(persistent)
	addOperatorFlags(persistent)

	addSinkRunFlags(sinkPostgresCmd.Flags(), sinkPostgresDriver)
	setModeGroupedUsage(sinkPostgresCmd)

	setupFlags := sinkPostgresSetupCmd.Flags()
	addCursorTableFlags(setupFlags)
	addOnModuleHashMismatchFlag(setupFlags)
	setupFlags.Bool("postgraphile", false, "Will append the necessary 'comments' on cursors table to fully support postgraphile")
	setupFlags.Bool("system-tables-only", false, "will only create/update the systems tables (cursors, substreams_history) and ignore the schema from the manifest")
	setupFlags.Bool("ignore-duplicate-table-errors", false, "[Dev] Use this if you want to ignore duplicate table errors, take caution that this means the 'schema.sql' file will not have run fully!")
	addBytesEncodingFlag(setupFlags)
	addFromProtoSchemaFlags(setupFlags)

	for _, constraintsCmd := range []*cobra.Command{sinkPostgresConstraintsApplyCmd, sinkPostgresConstraintsDropCmd} {
		addBytesEncodingFlag(constraintsCmd.Flags())
		addFromProtoSchemaFlags(constraintsCmd.Flags())
		addConstraintPassFlags(constraintsCmd.Flags())
	}
	sinkPostgresConstraintsCmd.AddCommand(sinkPostgresConstraintsApplyCmd)
	sinkPostgresConstraintsCmd.AddCommand(sinkPostgresConstraintsDropCmd)

	sinkPostgresCmd.AddCommand(sinkPostgresSetupCmd)
	sinkPostgresCmd.AddCommand(sinkPostgresConstraintsCmd)
	sinkPostgresCmd.AddCommand(newSinkToolsCmd(sinkPostgresDriver))

	SinkCmd.AddCommand(sinkPostgresCmd)
}
