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

	t.Run("a schema with no constraints at all still wants the block number index", func(t *testing.T) {
		database := setup(t, "missing_constraints_index_only", protosql.ConstraintPolicy{
			Timing:             protosql.ConstraintsManual,
			DisableForeignKeys: true,
			DisablePrimaryKeys: []string{protosql.AllTables},
			DisableUniques:     []string{protosql.AllTables},
		})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.NotEmpty(t, missing, "an output with no annotations declares no constraints, but the reorg path still deletes by _block_number_")
		for _, name := range missing {
			require.Contains(t, name, "_block_number_idx", "only the index is left to create")
		}
	})

	t.Run("nothing the flags leave out is missing", func(t *testing.T) {
		database := setup(t, "missing_constraints_disabled", protosql.ConstraintPolicy{
			Timing:                  protosql.ConstraintsManual,
			DisableForeignKeys:      true,
			DisablePrimaryKeys:      []string{protosql.AllTables},
			DisableUniques:          []string{protosql.AllTables},
			DisableBlockNumberIndex: true,
		})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.Empty(t, missing, "nothing the policy leaves out can be missing")
	})
}

// TestDbProtoConstraintsPerTransaction covers the pass committing as it goes.
//
// A single transaction around every index build and every foreign key validation is what
// turns a large schema into an OOM that loses the whole pass. Committing in batches bounds
// that, and what a killed run finished has to still be there — which is the same property
// that lets the pass be re-run against a schema that is partly constrained already.
func TestDbProtoConstraintsPerTransaction(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	open := func(t *testing.T, schemaName string, perTransaction int) protosql.Database {
		t.Helper()

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption: true,
			Constraints: protosql.ConstraintPolicy{
				Timing:         protosql.ConstraintsManual,
				PerTransaction: perTransaction,
			},
			UseTransactions: true,
			DecodeBatchSize: 1,
		}.Defaults()

		database, err := db_proto.SetupDatabaseSchema(ctx, postgresContainer.ConnectionString, schemaName, defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
		require.NoError(t, err)
		t.Cleanup(func() { database.Close(ctx) })

		return database
	}

	for _, perTransaction := range []int{1, 3} {
		t.Run(fmt.Sprintf("%d per transaction", perTransaction), func(t *testing.T) {
			schemaName := fmt.Sprintf("constraints_per_transaction_%d", perTransaction)
			createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)

			database := open(t, schemaName, perTransaction)

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
			require.NotEmpty(t, missing, "dropping has to actually remove them, indexes included")
			require.Contains(t, missing, fmt.Sprintf("%s.customers.customers_block_number_idx", schemaName))
		})
	}
}
