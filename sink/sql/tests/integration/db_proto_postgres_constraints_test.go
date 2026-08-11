package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresConstraintsBackfill covers what --with-constraints has to do on a
// schema that a previous run created without them.
//
// The sink info hash cannot tell those two schemas apart — it is computed over the DDL
// the dialect would emit, constraints included, either way — so turning the flag on has
// to add the missing constraints rather than assume the schema already matches. Running
// it again must then be a no-op rather than an "already exists" failure.
func TestDbProtoPostgresConstraintsBackfill(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()

	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	schemaName := "constraints_backfill"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)

	setup := func(useConstraints bool) {
		t.Helper()

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption:  true,
			UseConstraints:  useConstraints,
			UseTransactions: true,
			BlockBatchSize:  1,
		}.Defaults()

		database, err := db_proto.SetupDatabaseSchema(ctx, postgresContainer.ConnectionString, schemaName, defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
		require.NoError(t, err)
		require.NoError(t, database.Close(ctx))
	}

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	countConstraints := func(kinds string) int {
		t.Helper()

		var count int
		query := fmt.Sprintf(`
			SELECT count(*)
			FROM pg_constraint c
			JOIN pg_namespace n ON n.oid = c.connamespace
			WHERE n.nspname = $1 AND c.contype IN (%s)`, kinds)
		require.NoError(t, dbx.Get(&count, query, schemaName))

		return count
	}

	constraintExists := func(name string) bool {
		t.Helper()

		var count int
		require.NoError(t, dbx.Get(&count, `
			SELECT count(*)
			FROM pg_constraint c
			JOIN pg_namespace n ON n.oid = c.connamespace
			WHERE n.nspname = $1 AND c.conname = $2`, schemaName, name))

		return count > 0
	}

	// The static DDL gives _sink_info_ and _cursor_ inline primary keys, so the baseline
	// is not zero; what the dialect adds on top of it is what this is about.
	setup(false)
	baseline := countConstraints("'p','u','f'")
	require.False(t, constraintExists("block_pk"), "a schema created without constraints must not carry the dialect's own")

	setup(true)
	withConstraints := countConstraints("'p','u','f'")
	require.Greater(t, withConstraints, baseline, "--with-constraints must add the constraints to the existing schema")
	require.True(t, constraintExists("block_pk"), "the _blocks_ primary key")
	require.True(t, constraintExists("fk_block"), "the foreign key every table has to _blocks_")

	setup(true)
	require.Equal(t, withConstraints, countConstraints("'p','u','f'"), "applying the constraints again must not duplicate or fail on them")
}
