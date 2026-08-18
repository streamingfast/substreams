package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

type moduleRow struct {
	BlockNumber uint64 `db:"_block_number_"`
	RowID       uint32 `db:"_row_id_"`
	Name        string `db:"name"`
}

// TestDbProtoClickhouseWithoutAnnotations covers a package whose output proto carries no
// schema.proto annotations at all, which is what a Substreams written without the sink in
// mind looks like. Setup used to fail outright for want of 'order_by_fields'; the sink now
// sorts those tables on (_block_number_, _row_id_).
//
// The rows are what makes this worth running against a real server: the tables are
// ReplacingMergeTree, so a sorting key that does not tell a block's rows apart loses all
// but one of them at merge time. OPTIMIZE FINAL forces that merge rather than waiting for
// ClickHouse to get around to it.
func TestDbProtoClickhouseWithoutAnnotations(t *testing.T) {
	dbx, schema, dsn, options := runUnannotatedClickhouseSink(t, "no_annotations",
		blockScopedData(t, "1a", modulesOutput("map_one", "map_two", "map_three")),
	)

	var createTable string
	require.NoError(t, dbx.Get(&createTable, fmt.Sprintf("SHOW CREATE TABLE %s.Module", schema)))
	assert.Contains(t, createTable, "ORDER BY (_block_number_, _row_id_)")

	_, err := dbx.Exec(fmt.Sprintf("OPTIMIZE TABLE %s.Module FINAL", schema))
	require.NoError(t, err)

	var rows []moduleRow
	require.NoError(t, dbx.Select(&rows, fmt.Sprintf("SELECT _block_number_, _row_id_, name FROM %s.Module ORDER BY _row_id_", schema)))

	assert.Equal(t, []moduleRow{
		{1, 0, "map_one"},
		{1, 1, "map_two"},
		{1, 2, "map_three"},
	}, rows)

	// The guardrail: a database whose tables no longer agree with what the package would
	// create is refused rather than written into. A table without _row_id_ is what an
	// earlier run of an annotated package leaves behind, and CREATE TABLE IF NOT EXISTS
	// would keep it as it is.
	_, err = dbx.Exec(fmt.Sprintf("DROP TABLE %s.Module", schema))
	require.NoError(t, err)

	_, err = dbx.Exec(fmt.Sprintf(`CREATE TABLE %s.Module (
			_block_number_ UInt64, _block_timestamp_ timestamp, _version_ Int64, _deleted_ bool, name VARCHAR
		) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY (_block_number_)`, schema))
	require.NoError(t, err)

	_, err = db_proto.SetupDatabaseSchema(context.Background(), dsn, schema, defaultOutputModuleName,
		(&pbsubstreams.Modules{}).ProtoReflect().Descriptor(), options, logger, tracer)
	require.ErrorContains(t, err, "_row_id_")
}

// TestDbProtoClickhouseWithoutAnnotationsUndo checks the other half of the sorting key:
// an undo writes one tombstone per row, and a tombstone only removes anything if it lands
// on the very same key. _row_id_ is part of that key here, so it has to be carried into
// the tombstone as well.
func TestDbProtoClickhouseWithoutAnnotationsUndo(t *testing.T) {
	dbx, schema, _, _ := runUnannotatedClickhouseSink(t, "no_annotations_undo",
		blockScopedData(t, "1a", modulesOutput("map_one", "map_two")),
		blockScopedData(t, "2a", modulesOutput("map_three", "map_four")),
		blockUndo(t, "1a"),
	)

	var rows []moduleRow
	require.NoError(t, dbx.Select(&rows, fmt.Sprintf("SELECT _block_number_, _row_id_, name FROM %s.Module FINAL WHERE _deleted_ = 0 ORDER BY _block_number_, _row_id_", schema)))

	assert.Equal(t, []moduleRow{
		{1, 0, "map_one"},
		{1, 1, "map_two"},
	}, rows)
}

func modulesOutput(names ...string) *pbsubstreams.Modules {
	out := &pbsubstreams.Modules{}
	for _, name := range names {
		out.Modules = append(out.Modules, &pbsubstreams.Module{Name: name})
	}

	return out
}

// runUnannotatedClickhouseSink streams the given responses into a fresh ClickHouse
// database, with the schema derived from a proto carrying no annotations, and returns a
// handle on that database.
func runUnannotatedClickhouseSink(t *testing.T, schemaName string, responses ...*pbsubstreamsrpc.Response) (*sqlx.DB, string, string, db_proto.SinkerFactoryOptions) {
	t.Helper()

	outputMessageDescriptor := (&pbsubstreams.Modules{}).ProtoReflect().Descriptor()

	var dbDatabase string
	clickhouseDSN, _ := setupClickhouseContainer(t, func(ctx context.Context, user, password, database, dsn string, container *clickhouse.ClickHouseContainer) error {
		dbDatabase = database
		return nil
	})

	pattern := make([]interface{}, len(responses))
	for i, response := range responses {
		pattern[i] = response
	}

	substreamsClientConfig := setupFakeSubstreamsServer(t, pattern...)
	substreamsPackage := substreamsTestPackage(pbsubstreams.File_sf_substreams_v1_modules_proto, outputMessageDescriptor)

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

	clickhouseStateFolder := t.TempDir()
	options := db_proto.SinkerFactoryOptions{
		UseProtoOption:  false,
		Constraints:     protosql.DisableAllConstraints(),
		UseTransactions: true,
		DecodeBatchSize: 1,
		Clickhouse: db_proto.SinkerFactoryClickhouse{
			SinkInfoFolder: clickhouseStateFolder,
			CursorFilePath: filepath.Join(clickhouseStateFolder, "cursor.txt"),
		},
	}.Defaults()

	sinkerFactory := db_proto.SinkerFactory(baseSink, defaultOutputModuleName, outputMessageDescriptor, options)

	createTestDatabase(t, clickhouseDSN, schemaName)
	testDSN := strings.Replace(clickhouseDSN, dbDatabase, schemaName, 1)

	ctx := context.Background()
	dbSinker, err := sinkerFactory(ctx, testDSN, schemaName, logger, tracer)
	require.NoError(t, err)

	require.NoError(t, dbSinker.Run(ctx))
	require.NoError(t, dbSinker.Err())

	db, err := sql.Open("clickhouse", testDSN)
	require.NoError(t, err)

	// Unsafe: the rows read back leave _version_ and _deleted_ out.
	dbx := sqlx.NewDb(db, "clickhouse").Unsafe()
	t.Cleanup(func() { dbx.Close() })

	return dbx, schemaName, testDSN, options
}
