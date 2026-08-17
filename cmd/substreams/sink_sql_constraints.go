package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"go.uber.org/zap"
)

// fromProtoSchemaFlagNames are the from-proto flags that describe the schema itself, as
// opposed to how the sink writes to it. They are the ones `setup` and `constraints` need
// too: all three commands have to agree on which constraints the schema is meant to have.
var fromProtoSchemaFlagNames = []string{
	"disable-foreign-keys",
	"disable-primary-keys",
	"disable-unique-constraints",
	"disable-block-number-index",
	"no-constraints",
	"proto-file-override",
}

// addConstraintPassFlags registers what governs how the constraint pass runs, as opposed
// to which constraints it creates. Only the `constraints` commands carry it: the run
// creates them one at a time, which is the safe shape, and anything more deliberate is
// what the command is for.
func addConstraintPassFlags(flags *pflag.FlagSet) {
	flags.Int("constraints-parallelism", 1, "How many constraints are created or dropped at once. They go on independent relations, so the server can build them side by side, and the only ordering that matters is that a foreign key needs the key it references to exist — which the pass handles by running the keys first. One at a time by default, since each build takes its own --constraints-work-mem and holds a lock on its table. On a schema whose rows sit in one dominant table this buys little; on one with several large tables it is close to linear.")
	flags.String("constraints-work-mem", "", "What maintenance_work_mem is set to for the duration of each constraint statement, e.g. 1GB. Empty leaves the server's own setting alone, which on most is 64MB — at which an index build over a large table spills to an external merge sort. Raising it for the pass alone is the cheapest thing that makes it faster. It multiplies with --constraints-parallelism, each concurrent build taking its own.")

	flags.Int("constraints-per-transaction", 1, "Deprecated, use --constraints-parallelism.")
	_ = flags.MarkDeprecated("constraints-per-transaction", "use --constraints-parallelism")
}

// constraintsParallelism reads the parallelism, honouring the name it shipped under. The
// old flag was documented as an execution knob and only ever bundled statements into one
// transaction, which is not what it says.
func constraintsParallelism(cmd *cobra.Command) int {
	if flagChanged(cmd, "constraints-parallelism") {
		return intFlag(cmd, "constraints-parallelism")
	}
	if flagChanged(cmd, "constraints-per-transaction") {
		return intFlag(cmd, "constraints-per-transaction")
	}

	return intFlag(cmd, "constraints-parallelism")
}

func addFromProtoSchemaFlags(flags *pflag.FlagSet) {
	flags.Bool("disable-foreign-keys", false, "Never create foreign keys, including the one every table has to the block table. They are what a load pays most for, and without them a reorg is undone by deleting from each table rather than by a cascade.")
	flags.StringSlice("disable-primary-keys", nil, "Tables that go without a primary key, or 'all'. No primary key also means no index on the entity id.")
	flags.StringSlice("disable-unique-constraints", nil, "Tables whose unique constraints are left out, or 'all'.")
	flags.Bool("disable-block-number-index", false, "Leave out the index on _block_number_. Every table carries that column and every reorg deletes from every table by it, so without the index each undo is a sequential scan per table — a foreign key indexes its referenced side only. It is created when the sink starts, concurrently and outside the constraint pass: --apply-constraints describes the schema and is yours to schedule, where this one the sink depends on to undo a reorg. Measured over 10GiB it costs 2.6s to build and 2% of the table. Only dead weight on a run that can never reorg, such as --final-blocks-only.")
	flags.String("proto-file-override", "", "Override protobuf file to use instead of extracting from substreams package")

	flags.Bool("no-constraints", false, "Deprecated, use --disable-foreign-keys --disable-primary-keys=all --disable-unique-constraints=all.")
	_ = flags.MarkDeprecated("no-constraints", "use --disable-foreign-keys --disable-primary-keys=all --disable-unique-constraints=all")
}

// addConstraintTimingFlag registers --apply-constraints, which says when the constraints
// are created.
//
// It is on `setup` as well as the run. There it answers the one question `setup` can act
// on: 'always' creates them with the schema, while 'auto' and 'manual' both leave the
// tables bare, the first for the run to constrain when it reaches chain HEAD and the
// second for `constraints apply`.
func addConstraintTimingFlag(flags *pflag.FlagSet) {
	flags.String("apply-constraints", string(protosql.ConstraintsAuto), "When the schema's constraints are created: 'auto' has the sink create them once the stream reaches chain HEAD — and only there, a stop block ending a run without saying the backfill is done — 'manual' leaves it to the 'sink postgres constraints apply' command, 'always' creates them before the load. Creating them is a stop-the-world operation — indexes to build, foreign keys to validate, tables locked throughout — so on a large database 'manual' is how that pass goes into a maintenance window instead. Loading with them already in place is the expensive option: measured through binary COPY, 27x slower than loading without, where building the same constraints afterwards costs 3.3x. The index on _block_number_ is not one of these: the sink creates it when it starts, see --disable-block-number-index.")
}

// newSinkApplyConstraintsE creates the schema's constraints on a database the sink has
// already loaded, which is deliberately a separate command.
//
// It is a stop-the-world operation: every index is built and every foreign key validated,
// with the tables locked while it runs. On a large database that is a maintenance window,
// so it is the operator who picks the moment, not the sink.
// constraintsAction says which way `sink postgres constraints` runs.
type constraintsAction string

const (
	constraintsApply constraintsAction = "apply"
	constraintsDrop  constraintsAction = "drop"
)

func newSinkConstraintsE(driver string, action constraintsAction) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		dsnString, err := resolveSinkDSN(cmd)
		if err != nil {
			return err
		}
		if _, err := validateDSNEngine(dsnString, driver); err != nil {
			return err
		}

		manifestPath, outputModule := sinkManifestAndModule(args)

		sinkDBPreStart(cmd)
		sink.LoadSubstreamsAuthEnvFile(manifestPath)

		spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, outputModule, sink.IgnoreOutputModuleType, false, nil, zlog)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}

		if isDatabaseChangesType(module.Output.Type) {
			return errDatabaseChangesOwnsSchema("sink " + driver + " constraints")
		}

		constraints, err := fromProtoConstraintPolicy(cmd)
		if err != nil {
			return err
		}
		if constraints.SkipsEverything() {
			return fmt.Errorf("every constraint is disabled by the flags, so there is nothing to %s", action)
		}

		dsn, err := db.ParseDSN(dsnString)
		if err != nil {
			return fmt.Errorf("parsing dsn: %w", err)
		}

		protoFileOverride := sflags.MustGetString(cmd, "proto-file-override")
		rootMessageDescriptor, _, useProtoOption, err := resolveFromProtoRootMessage(spkg, module.Name, protoFileOverride)
		if err != nil {
			return err
		}
		if !useProtoOption {
			// Without annotations the sink has no declared keys or relations, but the
			// index it creates for its own reorg path is not declared either — so there is
			// still something to do, and refusing would leave every table unindexed.
			constraints = protosql.DisableAllConstraints().WithBlockNumberIndex(constraints)

			zlog.Warn("the module's output carries no schema.proto annotations, so there are no primary keys, unique constraints or foreign keys to " + string(action) + ". " +
				"The index on _block_number_ is not affected: the sink creates that one when it starts. " +
				"`substreams tools extract-proto --sql` writes the annotated proto to start from")
		}

		encoding, err := sinkBytesEncoding(cmd)
		if err != nil {
			return err
		}

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption: useProtoOption,
			Constraints:    constraints,
			Encoding:       encoding,
			Clickhouse:     fromProtoClickhouseOptions(cmd, driver),
		}

		if action == constraintsApply {
			zlog.Info("creating the schema's constraints, this locks every table while indexes are built and foreign keys validated",
				zap.String("schema", dsn.Schema()),
				zap.String("constraints", constraints.Describe()))
		} else {
			zlog.Info("dropping the schema's constraints",
				zap.String("schema", dsn.Schema()),
				zap.String("constraints", constraints.Describe()))
		}

		startAt := time.Now()
		database, err := db_proto.SetupDatabaseSchema(cmd.Context(), dsnString, dsn.Schema(), module.Name, rootMessageDescriptor.UnwrapMessage(), options, zlog, tracer)
		if err != nil {
			return fmt.Errorf("setting up database schema: %w", err)
		}
		defer database.Close(cmd.Context())

		if action == constraintsApply {
			if err := applyDatabaseConstraints(database); err != nil {
				return err
			}
			zlog.Info("constraints created", zap.Duration("duration", time.Since(startAt)))

			return nil
		}

		if err := dropDatabaseConstraints(database); err != nil {
			return err
		}
		zlog.Info("constraints dropped", zap.Duration("duration", time.Since(startAt)))

		return nil
	}
}

// applyDatabaseConstraints and dropDatabaseConstraints do not wrap the pass in a
// transaction: it owns its own, committing every --constraints-per-transaction statements
// so a killed run keeps what it finished.
func applyDatabaseConstraints(database protosql.Database) error {
	if err := database.ApplyConstraints(); err != nil {
		return fmt.Errorf("applying constraints: %w", err)
	}

	return nil
}

func dropDatabaseConstraints(database protosql.Database) error {
	if err := database.DropConstraints(); err != nil {
		return fmt.Errorf("dropping constraints: %w", err)
	}

	return nil
}

// fromProtoConstraintPolicy resolves the constraint flags. Everything is created by
// default, once the backfill is done rather than before it.
func fromProtoConstraintPolicy(cmd *cobra.Command) (protosql.ConstraintPolicy, error) {
	timing, err := protosql.ParseConstraintTiming(stringFlag(cmd, "apply-constraints"))
	if err != nil {
		return protosql.ConstraintPolicy{}, err
	}

	policy := protosql.ConstraintPolicy{
		Timing:             timing,
		DisableForeignKeys: boolFlag(cmd, "disable-foreign-keys"),
		DisablePrimaryKeys: stringSliceFlag(cmd, "disable-primary-keys"),
		DisableUniques:     stringSliceFlag(cmd, "disable-unique-constraints"),

		DisableBlockNumberIndex: boolFlag(cmd, "disable-block-number-index"),
		Parallelism:             constraintsParallelism(cmd),
		WorkMem:                 stringFlag(cmd, "constraints-work-mem"),
	}

	// --no-constraints shipped in v1.21.0 and said "none of them at all", which is exactly
	// what the three switches say together. It stays honoured for a release.
	if flagChanged(cmd, "no-constraints") && boolFlag(cmd, "no-constraints") {
		disabled := protosql.DisableAllConstraints()
		policy.DisableForeignKeys = true
		policy.DisablePrimaryKeys = disabled.DisablePrimaryKeys
		policy.DisableUniques = disabled.DisableUniques
	}

	return policy, nil
}

// warnAboutConstraints says out loud what applying constraints before the backfill costs,
// since the flag makes the load an order of magnitude slower and its effect on an already
// populated database is not instantaneous.
func warnAboutConstraints(constraints protosql.ConstraintPolicy) {
	if !constraints.ApplyUpfront() {
		return
	}

	zlog.Warn("constraints are being created before the backfill rather than after it. Loading with foreign keys in place measured 27x slower than loading without them, " +
		"where building the same constraints afterwards cost 3.3x, so a large initial sync is considerably slower this way. " +
		"Creating the constraints on an already populated database can also take a long time, and locks the tables while it runs")
}
