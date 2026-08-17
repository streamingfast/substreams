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

// TestDbProtoPostgresWriteModes runs the same blocks through every write mode and
// requires the resulting tables to be identical.
//
// All three now go through the spool, so what differs is only how a sealed segment is
// pushed: binary COPY files, multi-row INSERTs built from rendered tuples per table, or
// an interleaved log replayed one row at a time. A mode is only a performance choice if
// the rows it produces cannot be told apart, which is what this pins.
func TestDbProtoPostgresWriteModes(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	db, err := sql.Open("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	dbx := sqlx.NewDb(db, "postgres").Unsafe()
	defer dbx.Close()

	type row struct {
		CustomerId string `db:"customer_id"`
		Name       string `db:"name"`
	}

	read := func(schemaName string) []row {
		t.Helper()

		var rows []row
		require.NoError(t, dbx.Select(&rows, fmt.Sprintf(`SELECT customer_id, name FROM "%s"."customers" ORDER BY customer_id`, schemaName)))

		return rows
	}

	run := func(t *testing.T, schemaName string, mode protosql.WriteMode) []row {
		t.Helper()

		responses := []interface{}{
			relationsBlockData(t, "1a", "2025-01-01", entityCustomer("customer-1", "alpha")),
			relationsBlockData(t, "2a", "2025-01-02", entityCustomer("customer-2", "beta"), entityCustomer("customer-3", "gamma")),
			// A backslash and a quote: the rendered modes build SQL literals where COPY
			// hands pgtype the string as it is, so this is where the two would disagree.
			relationsBlockData(t, "3a", "2025-01-03", entityCustomer("customer-4", `delta\path 'quoted'`)),
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
			sink.WithBlockRange(bstream.MustParseRange("1-4", bstream.WithExclusiveEnd())),
			sink.WithRetryBackOff(&backoff.StopBackOff{}),
		)
		require.NoError(t, err)

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption:  true,
			Constraints:     protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual},
			UseTransactions: true,
			WriteMode:       mode,
			DecodeBatchSize: 1,
			// A tiny idle window so a run this short seals through the timer rather than
			// only at Close, which is the path a stalled stream takes.
			Spool: &spool.Options{Dir: t.TempDir(), MaxIdle: 100_000_000},
		}.Defaults()

		createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)

		dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)(ctx, postgresContainer.ConnectionString, schemaName, logger, tracer)
		require.NoError(t, err)

		require.NoError(t, dbSinker.Run(ctx))
		require.NoError(t, dbSinker.Err())

		return read(schemaName)
	}

	expected := []row{
		{CustomerId: "customer-1", Name: "alpha"},
		{CustomerId: "customer-2", Name: "beta"},
		{CustomerId: "customer-3", Name: "gamma"},
		{CustomerId: "customer-4", Name: `delta\path 'quoted'`},
	}

	for _, test := range []struct {
		schema string
		mode   protosql.WriteMode
	}{
		{"write_mode_copy", protosql.WriteModeCopy},
		{"write_mode_batch_insert", protosql.WriteModeBatchInsert},
		{"write_mode_row_insert", protosql.WriteModeRowInsert},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			require.Equal(t, expected, run(t, test.schema, test.mode))
		})
	}
}

// TestDbProtoPostgresWriteModeUnsupported covers the promise that an explicit mode the
// schema cannot support is an error rather than a downgrade.
//
// A cycle in the foreign keys has no table order, so the two table-grouped modes cannot
// keep a parent ahead of its children. Falling back silently is what used to happen, and
// it costs an order of magnitude with only a log line to say so.
func TestDbProtoPostgresWriteModeUnsupported(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()
	postgresContainer := sharedDbChangesPostgresContainer
	ctx := context.Background()

	schemaName := "write_mode_unsupported"
	createPostgresTestSchema(t, postgresContainer.ConnectionString, schemaName)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  true,
		Constraints:     protosql.ConstraintPolicy{Timing: protosql.ConstraintsManual},
		UseTransactions: true,
		WriteMode:       protosql.WriteMode("nonsense"),
		DecodeBatchSize: 1,
	}.Defaults()

	_, err := db_proto.SetupDatabaseSchema(ctx, postgresContainer.ConnectionString, schemaName, defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
	require.ErrorContains(t, err, "invalid write mode")
}
