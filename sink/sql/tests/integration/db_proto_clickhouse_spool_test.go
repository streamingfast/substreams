package tests

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

// TestDbProtoClickhouseSpool runs the same blocks with and without the spool and requires
// the same rows either way.
//
// ClickHouse gains nothing from the spool but the decoupling: the rows are replayed into
// the same column builders and sent by the same flush, followed by the same cursor write.
// So the spool is only worth having if it is invisible in the result, which is what this
// pins.
func TestDbProtoClickhouseSpool(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()

	var dbDatabase string
	clickhouseDSN, _ := setupClickhouseContainer(t, func(ctx context.Context, user, password, database, dsn string, container *clickhouse.ClickHouseContainer) error {
		dbDatabase = database
		return nil
	})

	run := func(t *testing.T, schemaName string, spooled bool) []*CustomerRow {
		t.Helper()

		responses := []interface{}{
			relationsBlockData(t, "1a", "2025-01-01", entityCustomer("customer-1", "alpha")),
			relationsBlockData(t, "2a", "2025-01-02", entityCustomer("customer-2", "beta"), entityCustomer("customer-3", "gamma")),
			relationsBlockData(t, "3a", "2025-01-03", entityCustomer("customer-4", "delta")),
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

		stateFolder := t.TempDir()

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption:  true,
			Constraints:     protosql.DisableAllConstraints(),
			UseTransactions: true,
			DecodeBatchSize: 1,
			Clickhouse: db_proto.SinkerFactoryClickhouse{
				SinkInfoFolder: stateFolder,
				CursorFilePath: filepath.Join(stateFolder, "cursor.txt"),
			},
		}
		if spooled {
			options.Spool = &spool.Options{Dir: t.TempDir(), MaxIdle: 100_000_000}
		}

		createTestDatabase(t, clickhouseDSN, schemaName)
		testDSN := strings.Replace(clickhouseDSN, dbDatabase, schemaName, 1)

		ctx := context.Background()
		dbSinker, err := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options.Defaults())(ctx, testDSN, schemaName, logger, tracer)
		require.NoError(t, err)

		require.NoError(t, dbSinker.Run(ctx))
		require.NoError(t, dbSinker.Err())

		db, err := sql.Open("clickhouse", testDSN)
		require.NoError(t, err)
		dbx := sqlx.NewDb(db, "clickhouse").Unsafe()
		defer dbx.Close()

		return readRowsBy[CustomerRow](t, dbx, "customers", "customer_id")
	}

	direct := run(t, "clickhouse_direct", false)
	spooled := run(t, "clickhouse_spooled", true)

	require.NotEmpty(t, direct, "the unspooled run has to write something for the comparison to mean anything")
	require.Equal(t, direct, spooled)
}

// CustomerRow is one row of the customers table, for comparing a spooled run against an
// unspooled one.
type CustomerRow struct {
	Meta
	CustomerID string `db:"customer_id"`
	Name       string `db:"name"`
}
