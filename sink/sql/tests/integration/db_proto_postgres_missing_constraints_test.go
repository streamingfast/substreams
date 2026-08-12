package tests

import (
	"context"
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
		require.Subset(t, missing, []string{"block_pk", "fk_block", "customers_pk"})
	})

	t.Run("nothing is missing once they have been created", func(t *testing.T) {
		database := setup(t, "missing_constraints_applied", protosql.ConstraintPolicy{Timing: protosql.ConstraintsAlways})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.Empty(t, missing)
	})

	t.Run("a constraint the flags disable is not missing", func(t *testing.T) {
		database := setup(t, "missing_constraints_disabled", protosql.ConstraintPolicy{
			Timing:             protosql.ConstraintsManual,
			DisableForeignKeys: true,
			DisablePrimaryKeys: []string{protosql.AllTables},
			DisableUniques:     []string{protosql.AllTables},
		})

		missing, err := database.MissingConstraints()
		require.NoError(t, err)
		require.Empty(t, missing, "nothing the policy leaves out can be missing")
	})
}
