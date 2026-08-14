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
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
)

// TestDbProtoConstraintsAtEndOfRangeWithSpool covers the end of a bounded run that spooled.
//
// The spool has to be drained — the rows it holds are part of the range — but the
// constraints are deliberately left alone. A stop block ends the run without saying the
// backfill is over: a range is routinely one chunk of several, and building them here would
// leave every later chunk loading into a constrained schema, measured at 27.7x.
func TestDbProtoConstraintsAtEndOfRangeWithSpool(t *testing.T) {
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
		Spool:           &spool.Options{Dir: t.TempDir()},
	}.Defaults()

	testSchema := "constraints_end_of_range_spooled"
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
		require.Zero(t, count, "reaching a stop block must leave %q to `constraints apply`", name)
	}

	var customers int
	require.NoError(t, dbx.Get(&customers, fmt.Sprintf(`SELECT count(*) FROM "%s"."customers"`, testSchema)))
	require.Equal(t, 2, customers, "the spool is still drained at the end of the range, constraints or not")
}
