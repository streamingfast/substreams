package tests

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresBlockNumberIndex covers the index the sink creates for its own reorg
// path, which is deliberately not part of the constraint pass.
//
// --apply-constraints describes the schema and is the operator's to schedule. This one the
// sink depends on to undo a reorg without sequentially scanning every table, so it goes in
// when the sink starts whatever the constraints say — and concurrently, so a restart onto
// an already-loaded table neither waits for a lock nor takes one.
func TestDbProtoPostgresBlockNumberIndex(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	// validIndexes counts only the ones a query would actually use: an interrupted
	// concurrent build leaves an index behind that is there and unusable.
	validIndexes := func(schemaName string) []string {
		t.Helper()

		var names []string
		require.NoError(t, dbx.Select(&names, `
			SELECT cl.relname
			FROM pg_index i
			JOIN pg_class cl ON cl.oid = i.indexrelid
			JOIN pg_namespace n ON n.oid = cl.relnamespace
			WHERE n.nspname = $1 AND i.indisvalid AND cl.relname LIKE '%_block_number_idx'
			ORDER BY cl.relname`, schemaName))

		return names
	}

	open := func(t *testing.T, schemaName string, constraints protosql.ConstraintPolicy) protosql.Database {
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

	t.Run("created even when every constraint is disabled", func(t *testing.T) {
		schemaName := "block_index_no_constraints"
		database := open(t, schemaName, protosql.DisableAllConstraints())

		require.Empty(t, validIndexes(schemaName), "the schema starts without them")
		require.NoError(t, database.EnsureBlockNumberIndexes(ctx))

		created := validIndexes(schemaName)
		require.NotEmpty(t, created, "the reorg path deletes by _block_number_ whatever the constraints declare")
		require.Contains(t, created, "customers_block_number_idx")

		// Starting again has to be a no-op rather than an "already exists" failure.
		require.NoError(t, database.EnsureBlockNumberIndexes(ctx))
		require.Equal(t, created, validIndexes(schemaName))
	})

	t.Run("left out when the flag asks", func(t *testing.T) {
		schemaName := "block_index_disabled"
		policy := protosql.DisableAllConstraints()
		policy.DisableBlockNumberIndex = true

		database := open(t, schemaName, policy)

		require.NoError(t, database.EnsureBlockNumberIndexes(ctx))
		require.Empty(t, validIndexes(schemaName))
	})

	t.Run("the constraint pass neither creates nor drops it", func(t *testing.T) {
		schemaName := "block_index_not_a_constraint"
		database := open(t, schemaName, protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual})

		require.NoError(t, database.ApplyConstraints())
		require.Empty(t, validIndexes(schemaName), "the constraint pass runs in transactions, which a concurrent build cannot")

		require.NoError(t, database.EnsureBlockNumberIndexes(ctx))
		created := validIndexes(schemaName)
		require.NotEmpty(t, created)

		require.NoError(t, database.DropConstraints())
		require.Equal(t, created, validIndexes(schemaName), "dropping the constraints leaves the sink's own index alone")
	})
}
