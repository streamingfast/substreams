package tests

import (
	"context"
	"fmt"
	"testing"

	_ "github.com/lib/pq"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresMissingConstraints covers the startup check.
//
// A run interrupted before the backfill ended, or whose constraint pass was killed
// part-way, leaves a database that answers queries slowly and rejects nothing — and looks
// exactly like a database that is fine. The sink has to be able to say so, which means
// knowing which constraints the policy asks for and which the catalog actually has.
func TestDbProtoPostgresMissingConstraints(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	setup := func(t *testing.T, schemaName string, constraints protosql.ConstraintPolicy) protosql.Database {
		t.Helper()

		createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption:  true,
			Constraints:     constraints,
			UseTransactions: true,
			DecodeBatchSize: 1,
		}.Defaults()

		database, err := db_proto.SetupDatabaseSchema(ctx, postgresContainer.ConnectionString, schemaName, defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
		require.NoError(t, err)
		t.Cleanup(func() { database.Close(ctx) })

		return database
	}

	t.Run("a schema loaded bare reports every constraint missing", func(t *testing.T) {
		database := setup(t, "missing_constraints_bare", protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.Subset(t, missing, []string{
			"missing_constraints_bare._blocks_.block_pk",
			"missing_constraints_bare.customers.customers_pk",
			"missing_constraints_bare.customers.fk_block",
			"missing_constraints_bare.orders.fk_block",
		}, "each one is named by its relation as well: fk_block is a different constraint on every table")
	})

	t.Run("nothing is missing once they have been created", func(t *testing.T) {
		database := setup(t, "missing_constraints_applied", protosql.ConstraintPolicy{Timing: protosql.ConstraintsAlways})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.Empty(t, missing)
	})

	t.Run("nothing the flags leave out is missing", func(t *testing.T) {
		database := setup(t, "missing_constraints_disabled", protosql.ConstraintPolicy{
			Timing:             protosql.ConstraintsManual,
			DisableForeignKeys: true,
			DisablePrimaryKeys: []string{protosql.AllTables},
			DisableUniques:     []string{protosql.AllTables},
		})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.Empty(t, missing, "nothing the policy leaves out can be missing, and the block number index is not a constraint")
	})
}

// TestDbProtoConstraintsParallelism covers the pass at every width it can be run at.
//
// Each statement commits on its own whatever the parallelism, so what a killed run
// finished has to still be there — the same property that lets the pass be re-run against
// a schema that is partly constrained already. Running the keys and the foreign keys as
// separate waves is what keeps a foreign key from reaching the server before the key it
// references, which concurrency would otherwise make a race.
func TestDbProtoConstraintsParallelism(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	open := func(t *testing.T, schemaName string, parallelism int) protosql.Database {
		t.Helper()

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption: true,
			Constraints: protosql.ConstraintPolicy{
				Timing:      protosql.ConstraintsManual,
				Parallelism: parallelism,
				WorkMem:     "64MB",
			},
			UseTransactions: true,
			DecodeBatchSize: 1,
		}.Defaults()

		database, err := db_proto.SetupDatabaseSchema(ctx, postgresContainer.ConnectionString, schemaName, defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
		require.NoError(t, err)
		t.Cleanup(func() { database.Close(ctx) })

		return database
	}

	for _, parallelism := range []int{1, 3, 10} {
		t.Run(fmt.Sprintf("parallelism %d", parallelism), func(t *testing.T) {
			schemaName := fmt.Sprintf("constraints_parallelism_%d", parallelism)
			createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)

			database := open(t, schemaName, parallelism)

			require.NoError(t, database.ApplyConstraints())
			missing, err := database.MissingConstraints()
			require.NoError(t, err)
			require.Empty(t, missing)

			// Re-running has to be a no-op rather than an "already exists" failure, which
			// is what a resumed pass depends on.
			require.NoError(t, database.ApplyConstraints())

			require.NoError(t, database.DropConstraints())
			missing, err = database.MissingConstraints()
			require.NoError(t, err)
			require.NotEmpty(t, missing, "dropping has to actually remove them")
		})
	}
}
