package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoPostgresConstraintsBackfill covers what applying constraints has to do on a
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
			Constraints:     constraintPolicy(useConstraints),
			UseTransactions: true,
			DecodeBatchSize: 1,
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
	require.Greater(t, withConstraints, baseline, "applying constraints must add them to the existing schema")
	require.True(t, constraintExists("block_pk"), "the _blocks_ primary key")
	require.True(t, constraintExists("fk_block"), "the foreign key every table has to _blocks_")

	setup(true)
	require.Equal(t, withConstraints, countConstraints("'p','u','f'"), "applying the constraints again must not duplicate or fail on them")
}

// TestDbProtoConstraintsAtHead covers --apply-constraints=auto: the sink loads bare and
// creates the constraints itself once the backfill is over, which for a bounded run is
// the end of the range.
//
// The default is deliberately not this — building them locks every table, which is the
// operator's call to schedule — but when it is asked for it has to actually happen.
func TestDbProtoConstraintsAtHead(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	responses := []interface{}{
		relationsBlockData(t, "1a", "2025-01-01", entityCustomer("customer-1", "kept")),
		relationsBlockData(t, "2a", "2025-01-02", entityCustomer("customer-2", "kept")),
	}

	substreamsClientConfig := setupFakeSubstreamsServer(t, responses...)
	substreamsPackage := substreamsTestPackage(pbrelations.File_test_relations_relations_proto, outputMessageDescriptor)

	baseSink, err := sink.New(
		sink.SubstreamsModeProduction,
		false,
		substreamsPackage,
		substreamsPackage.Modules.Modules[0],
		manifest.ModuleHash{},
		substreamsClientConfig,
		logger,
		tracer,
		sink.WithBlockRange(bstream.MustParseRange("1-3", bstream.WithExclusiveEnd())),
		sink.WithRetryBackOff(&backoff.StopBackOff{}),
	)
	require.NoError(t, err)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		Constraints:     protosql.ConstraintPolicy{Timing: protosql.ConstraintsAuto},
		UseTransactions: true,
		DecodeBatchSize: 1,
	}.Defaults()

	testSchema := "constraints_at_head"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, testSchema)

	dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, testSchema, logger, tracer)
	require.NoError(t, err)

	require.NoError(t, dbSinker.Run(ctx))
	require.NoError(t, dbSinker.Err())

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	for _, name := range []string{"block_pk", "fk_block", "customers_pk"} {
		var count int
		require.NoError(t, dbx.Get(&count, `
			SELECT count(*)
			FROM pg_constraint c
			JOIN pg_namespace n ON n.oid = c.connamespace
			WHERE n.nspname = $1 AND c.conname = $2`, testSchema, name))
		require.Positive(t, count, "the sink must have created %q at the end of the run", name)
	}

	var customers int
	require.NoError(t, dbx.Get(&customers, fmt.Sprintf(`SELECT count(*) FROM "%s"."customers"`, testSchema)))
	require.Equal(t, 2, customers, "the rows loaded before the constraints are still there")
}
