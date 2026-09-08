package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	"github.com/streamingfast/bstream"
	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	"github.com/streamingfast/substreams/manifest"
	pbsql "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/services/v1"
	sink "github.com/streamingfast/substreams/sink"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestClickhouseSinker_Integration_SinglePrimaryKey(t *testing.T) {
	tests := []sinkerTestCase{
		{
			"insert final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1"),
				),
			),
			equalsClickhouseXferRows([]*XferSinglePKRow{
				{ID: "1234", From: "sender1", To: "receiver1"},
			}),
			"Block #10 (10a) - LIB #10 (10a)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runClickhouseSinkerTest(
				t,
				sharedDbChangesClickhouseContainer,
				nil,
				nil,
				tablesInput(func(schema string) map[string]*db2.TableInfo { return db2.TestSinglePrimaryKeyTables(schema) }),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func TestClickhouseSinker_Integration_AggregateFunction(t *testing.T) {
	runClickhouseSinkerTest(
		t,
		sharedDbChangesClickhouseContainer,
		nil,
		nil,
		rawSQLInput(func(schema string) string {
			return `
				CREATE TABLE IF NOT EXISTS metrics (
					id String,
					value String,
					count String,
					sum_value AggregateFunction(sum, Float64),
					avg_value AggregateFunction(avg, Float64),
					uniq_count AggregateFunction(uniq, String)
				) ENGINE = AggregatingMergeTree()
				ORDER BY id;
			`
		}),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowSinglePK("metrics", "metric1", "value", "100", "count", "1"),
			),
		),
		equalsClickhouseMetricsRows([]*MetricsRow{
			{ID: "metric1", Value: "100", Count: "1"},
		}),
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func TestClickhouseSinker_Integration_MaterializedView(t *testing.T) {
	runClickhouseSinkerTest(
		t,
		sharedDbChangesClickhouseContainer,
		nil,
		nil,
		rawSQLInput(func(schema string) string {
			return `
				CREATE TABLE IF NOT EXISTS xfer (
					id String,
					"from" String,
					"to" String
				) ENGINE = ReplacingMergeTree()
				ORDER BY id;

				CREATE MATERIALIZED VIEW IF NOT EXISTS xfer_summary
				ENGINE = SummingMergeTree()
				ORDER BY "from"
				AS SELECT
					"from",
					count() as transfer_count
				FROM xfer
				GROUP BY "from";
			`
		}),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1"),
			),
		),
		equalsClickhouseXferRows([]*XferSinglePKRow{
			{ID: "1234", From: "sender1", To: "receiver1"},
		}),
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func TestClickhouseSinker_Integration_MaterializedColumn(t *testing.T) {
	runClickhouseSinkerTest(
		t,
		sharedDbChangesClickhouseContainer,
		nil,
		nil,
		rawSQLInput(func(schema string) string {
			return `
				CREATE TABLE IF NOT EXISTS events (
					id String,
					timestamp DateTime,
					value String,
					minute DateTime MATERIALIZED toStartOfMinute(timestamp)
				) ENGINE = MergeTree()
				ORDER BY id;
			`
		}),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowSinglePK("events", "event1", "timestamp", "2021-01-01 12:34:56", "value", "test_value"),
			),
		),
		equalsClickhouseEventsRows([]*EventsRow{
			{ID: "event1", Timestamp: "2021-01-01T12:34:56Z", Value: "test_value"},
		}),
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func runClickhouseSinkerTest(
	t *testing.T,
	clickhouseContainer *ClickhouseContainerExt,
	customizeSinkOptions func(defaults []sink.Option) []sink.Option,
	customizeSinkerFactoryOptions func(defaults sinker.SinkerFactoryOptions) sinker.SinkerFactoryOptions,
	setupInput sinkerSetupInput,
	responses []any,
	expected func(t *testing.T, dbx *sqlx.DB, schema string),
	expectedFinalCursor string,
) {
	t.Helper()
	t.Parallel()

	ctx := context.Background()

	// Generate a random database name instead of using schemaName
	databaseName := NewRandomSchemaName()

	// First connect with original connection string to create the database
	originalDSN := clickhouseContainer.ConnectionString
	tempDB, connectErr := sqlx.Connect("clickhouse", originalDSN)
	require.NoError(t, connectErr, "connecting to ClickHouse")

	// Create the new database
	_, createErr := tempDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", databaseName))
	require.NoError(t, createErr, "creating database")
	tempDB.Close()

	// Parse original DSN and modify it to use the new database
	parsedDSN, parseErr := db2.ParseDSN(originalDSN)
	require.NoError(t, parseErr, "parsing original DSN")

	// Update the database field and build new connection string
	parsedDSN.Database = databaseName.Unescaped
	dsnRaw := parsedDSN.ConnString()

	// For backward compatibility, keep using schemaName variable name but it's now the database name
	schemaName := databaseName

	substreamsClientConfig := setupFakeSubstreamsServer(t, responses...)
	spkg := substreamsTestPackage(pbdatabase.File_sf_substreams_sink_database_v1_database_proto, (*pbdatabase.DatabaseChanges)(nil).ProtoReflect().Descriptor())

	var err error
	spkg.SinkConfig, err = anypb.New(&pbsql.Service{
		Schema: setupInput.ToSQL(schemaName.Unescaped),
	})
	require.NoError(t, err)

	setupOptions := sinker.SinkerSetupOptions{
		CursorTableName:            "cursors",
		HistoryTableName:           "history",
		ClickhouseCluster:          "",
		OnModuleHashMismatch:       "error",
		SystemTablesOnly:           false,
		IgnoreDuplicateTableErrors: false,
		Postgraphile:               false,
	}

	err = sinker.SinkerSetup(ctx, dsnRaw, spkg, setupOptions, logger, tracer)
	require.NoError(t, err, "setting up sinker")

	baseSinkOptions := []sink.Option{
		sink.WithBlockRange(bstream.MustParseRange("1-1000", bstream.WithExclusiveEnd())),
		sink.WithLivenessChecker(&isAlwaysLiveChecker{}),
		sink.WithRetryBackOff(&backoff.StopBackOff{}),
	}
	if customizeSinkOptions != nil {
		baseSinkOptions = customizeSinkOptions(baseSinkOptions)
	}

	// Load table metadata (including cursor table) - this is required for InsertCursor to work
	baseSink, err := sink.New(
		sink.SubstreamsModeProduction,
		false,
		spkg,
		spkg.Modules.Modules[0],
		manifest.ModuleHash("test_module_hash"),
		substreamsClientConfig,
		logger,
		tracer,
		baseSinkOptions...,
	)
	require.NoError(t, err)

	// Create sinker factory options - force immediate flushing for testing
	options := sinker.SinkerFactoryOptions{
		CursorTableName:         setupOptions.CursorTableName,
		HistoryTableName:        setupOptions.HistoryTableName,
		ClickhouseCluster:       setupOptions.ClickhouseCluster,
		BatchBlockFlushInterval: 1, // Force flush every 1 block
		BatchRowFlushInterval:   1, // Force flush every 1 row
		LiveBlockFlushInterval:  1, // Force flush every 1 block for live data
		OnModuleHashMismatch:    setupOptions.OnModuleHashMismatch,
		HandleReorgs:            false, // ClickHouse doesn't support reorg handling
		FlushRetryCount:         0,
		FlushRetryDelay:         0,
	}

	if customizeSinkerFactoryOptions != nil {
		options = customizeSinkerFactoryOptions(options)
	}

	dbSinker, err := sinker.SinkerFactory(baseSink, options)(ctx, dsnRaw, logger, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { dbSinker.Close() })

	dbSinker.Run(ctx)
	require.NoError(t, dbSinker.Err())

	if expected != nil {
		// Parse DSN to get clean connection string without schemaName
		parsedDSN, err := db2.ParseDSN(dsnRaw)
		require.NoError(t, err)

		// Connect to database and check results
		dbx, err := sqlx.Connect("clickhouse", parsedDSN.ConnString())
		require.NoError(t, err)
		defer dbx.Close()

		expected(t, dbx, schemaName.Unescaped)
	}

	if expectedFinalCursor != "" {
		// Parse DSN to get clean connection string without schemaName
		parsedDSN, err := db2.ParseDSN(dsnRaw)
		require.NoError(t, err)

		// Connect to database to read cursor directly
		db, err := sqlx.Connect("clickhouse", parsedDSN.ConnString())
		require.NoError(t, err)
		defer db.Close()

		// Fetch cursor directly from database with retry for ClickHouse timing
		cursorStr := waitForClickHouseCursor(t, ctx, db, schemaName, dbSinker.OutputModuleHash())

		finalCursor, err := bstream.CursorFromOpaque(cursorStr)
		require.NoError(t, err)

		actualCursor := fmt.Sprintf("Block %s - LIB %s", finalCursor.Block, finalCursor.LIB)
		require.Equal(t, expectedFinalCursor, actualCursor)
	}
}

// EscapeClickhouseIdentifier escapes ClickHouse identifiers (kept for backward compatibility)
func EscapeClickhouseIdentifier(name string) string {
	return "`" + name + "`"
}

func equalsClickhouseXferRows(expected []*XferSinglePKRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		schemaName := NewSchemaName(schema)
		require.Equal(t, expected, readClickhouseDbChangesRows[XferSinglePKRow](t, dbx, schemaName, "xfer"))
	}
}

func readClickhouseDbChangesRows[T any](t *testing.T, db *sqlx.DB, schema SchemaName, table string) []*T {
	t.Helper()

	tableName := NewSchemaName(table) // Reusing SchemaName for table identifier escaping

	var rows []*T
	err := db.SelectContext(context.Background(), &rows, fmt.Sprintf(`SELECT * FROM %s.%s ORDER BY id;`, schema, tableName))
	require.NoError(t, err)

	return rows
}

// MetricsRow represents a row in the metrics table with aggregate functions
type MetricsRow struct {
	ID    string `db:"id"`
	Value string `db:"value"`
	Count string `db:"count"`
}

func equalsClickhouseMetricsRows(expected []*MetricsRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		schemaName := NewSchemaName(schema)
		require.Equal(t, expected, readClickhouseMetricsRows(t, dbx, schemaName))
	}
}

func readClickhouseMetricsRows(t *testing.T, db *sqlx.DB, schema SchemaName) []*MetricsRow {
	t.Helper()

	var rows []*MetricsRow
	// Only select the regular String columns, not the AggregateFunction columns
	query := fmt.Sprintf(`SELECT id, value, count FROM %s.metrics ORDER BY id;`, schema)
	err := db.SelectContext(context.Background(), &rows, query)
	require.NoError(t, err)

	return rows
}

// EventsRow represents a row in the events table with MATERIALIZED columns
type EventsRow struct {
	ID        string `db:"id"`
	Timestamp string `db:"timestamp"`
	Value     string `db:"value"`
}

func equalsClickhouseEventsRows(expected []*EventsRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		schemaName := NewSchemaName(schema)
		require.Equal(t, expected, readClickhouseEventsRows(t, dbx, schemaName))
	}
}

func readClickhouseEventsRows(t *testing.T, db *sqlx.DB, schema SchemaName) []*EventsRow {
	t.Helper()

	var rows []*EventsRow
	// Only select the regular columns, not the MATERIALIZED columns
	query := fmt.Sprintf(`SELECT id, timestamp, value FROM %s.events ORDER BY id;`, schema)
	err := db.SelectContext(context.Background(), &rows, query)
	require.NoError(t, err)

	return rows
}

// waitForClickHouseCursor implements aggressive retry mechanism for ClickHouse cursor reads
// Uses OPTIMIZE TABLE ... FINAL and retries 500 times with 10ms intervals (5 seconds total)
func waitForClickHouseCursor(t *testing.T, ctx context.Context, db *sqlx.DB, schema SchemaName, moduleHash string) string {
	t.Helper()

	// Force ClickHouse to merge all parts immediately to make data available
	optimizeQuery := fmt.Sprintf("OPTIMIZE TABLE %s.cursors FINAL", schema)
	_, err := db.ExecContext(ctx, optimizeQuery)
	require.NoError(t, err)

	cursorQuery := fmt.Sprintf("SELECT cursor FROM %s.cursors WHERE id = ?", schema)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	attempt := 0
	for range ticker.C {
		attempt++

		var cursorStr string
		err := db.GetContext(ctx, &cursorStr, cursorQuery, moduleHash)
		require.NoError(t, err)

		if cursorStr != "" {
			logger.Debug("retrieved cursor after attempts", zap.Int("attempt", attempt), zap.String("cursor", cursorStr))
			return cursorStr
		}

		if attempt == 1 || attempt%50 == 0 {
			logger.Debug("cursor retrieval attempt", zap.Int("attempt", attempt), zap.Int("max_attempts", 500), zap.String("cursor", cursorStr), zap.Error(err))
		}

		// Retry up to 500 times with 10ms intervals for aggressive timing handling
		if attempt >= 500 {
			break
		}
	}

	// Final attempt - let it fail with the actual error
	var cursorStr string
	err = db.GetContext(ctx, &cursorStr, cursorQuery, moduleHash)
	require.NoError(t, err, "Failed to retrieve cursor after 250 attempts")

	return cursorStr
}
