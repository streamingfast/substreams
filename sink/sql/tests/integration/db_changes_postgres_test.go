package tests

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/cli"
	sink "github.com/streamingfast/substreams/sink"
	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	pbsql "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/services/v1"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

var sharedDbChangesPostgresContainer *PostgresContainerExt
var sharedDbChangesClickhouseContainer *ClickhouseContainerExt

func TestMain(m *testing.M) {
	// These tests spin up Postgres and ClickHouse containers, opt-in only so
	// plain `go test ./...` works on machines without Docker (e.g. macOS CI)
	if os.Getenv("SF_SINK_SQL_INTEGRATION_TESTS") == "" {
		fmt.Println("skipping sink/sql integration tests, set SF_SINK_SQL_INTEGRATION_TESTS=true to run them")
		os.Exit(0)
	}

	var pgCleanup, chCleanup func()

	// Setup both containers in parallel
	var wg sync.WaitGroup
	wg.Add(2)

	// Setup PostgreSQL container
	go func() {
		defer wg.Done()
		sharedDbChangesPostgresContainer, pgCleanup = setupRawPostgresContainer(PostgresContainerConfig{
			Image: "postgres:18-alpine",
		})
	}()

	// Setup ClickHouse container
	go func() {
		defer wg.Done()
		sharedDbChangesClickhouseContainer, chCleanup = setupRawClickhouseContainer(ClickhouseContainerConfig{
			Image: "clickhouse/clickhouse-server:26.1-alpine",
		})
	}()

	// Wait for both containers to be ready
	wg.Wait()

	exitCode := m.Run()

	// Cleanup both containers
	pgCleanup()
	chCleanup()

	os.Exit(exitCode)
}

func TestSinker_Integration_SinglePrimaryKey(t *testing.T) {
	tests := []sinkerTestCase{
		{
			"insert final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1"),
				),
			),
			equalsXferRows([]*XferSinglePKRow{
				{ID: "1234", From: "sender1", To: "receiver1"},
			}),
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"insert then undo insertion",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1"),
				),
				blockUndo(t, "9a", finalBlock("8a")),
			),
			nil,
			"Block #9 (9a) - LIB #8 (8a)",
		},

		{
			"upsert final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("xfer", "1234", "from", "sender2", "to", "receiver2"),
				),
			),
			equalsXferRows([]*XferSinglePKRow{
				{ID: "1234", From: "sender2", To: "receiver2"},
			}),
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"upsert, first insert, second update, final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowSinglePK("xfer", "1234", "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("xfer", "1234", "to", "receiver3"),
				),
			),
			equalsXferRows([]*XferSinglePKRow{
				{ID: "1234", From: "sender2", To: "receiver3"},
			}),
			"Block #11 (11a) - LIB #11 (11a)",
		},

		{
			"upsert, first insert, undo insertion",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowSinglePK("xfer", "1234", "from", "sender2", "to", "receiver2"),
				),
				blockUndo(t, "9a", finalBlock("8a")),
			),
			nil,
			"Block #9 (9a) - LIB #8 (8a)",
		},
		{
			"upsert, first insert, second update, undo initial insert",
			streamMock(
				dbChangesBlockData(t, "10a", "8a",
					upsertRowSinglePK("xfer", "1234", "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", "8a",
					upsertRowSinglePK("xfer", "1234", "to", "receiver3"),
				),
				blockUndo(t, "9a", finalBlock("8a")),
			),
			nil,
			"Block #9 (9a) - LIB #8 (8a)",
		},
		{
			"upsert, first insert, second update, undo last update",
			streamMock(
				dbChangesBlockData(t, "10a", "8a",
					upsertRowSinglePK("xfer", "1234", "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", "8a",
					upsertRowSinglePK("xfer", "1234", "to", "receiver3"),
				),
				blockUndo(t, "10a", finalBlock("8a")),
			),
			equalsXferRows([]*XferSinglePKRow{
				{ID: "1234", From: "sender2", To: "receiver2"},
			}),
			"Block #10 (10a) - LIB #8 (8a)",
		},
		{
			"upsert, first insert, second update, third update, undo last update",
			streamMock(
				dbChangesBlockData(t, "10a", "8a",
					upsertRowSinglePK("xfer", "1234", "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", "8a",
					upsertRowSinglePK("xfer", "1234", "to", "receiver3"),
				),
				dbChangesBlockData(t, "12a", "8a",
					upsertRowSinglePK("xfer", "1234", "from", "sender3"),
				),
				blockUndo(t, "11a", finalBlock("8a")),
			),
			equalsXferRows([]*XferSinglePKRow{
				{ID: "1234", From: "sender2", To: "receiver3"},
			}),
			"Block #11 (11a) - LIB #8 (8a)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
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

func TestSinker_Integration_CompositePrimaryKey(t *testing.T) {
	pk := compositePK

	tests := []sinkerTestCase{
		// Insert testing

		{
			"insert final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowCompositePK("xfer", pk("id", "12", "number", "34"), "from", "sender1", "to", "receiver1"),
				),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "12", Number: "34", From: "sender1", To: "receiver1"},
			}),
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"insert then undo insertion",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					insertRowCompositePK("xfer", pk("id", "12", "number", "34"), "from", "sender1", "to", "receiver1"),
				),
				blockUndo(t, "9a", finalBlock("8a")),
			),
			nil,
			"Block #9 (9a) - LIB #8 (8a)",
		},

		// Upsert testing

		{
			"upsert final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender2", "to", "receiver2"),
				),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "12", Number: "34", From: "sender2", To: "receiver2"},
			}),
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"upsert, first insert, second update, final",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "to", "receiver3"),
				),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "12", Number: "34", From: "sender2", To: "receiver3"},
			}),
			"Block #11 (11a) - LIB #11 (11a)",
		},

		{
			"upsert, first insert, undo insertion",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender2", "to", "receiver2"),
				),
				blockUndo(t, "9a", finalBlock("8a")),
			),
			nil,
			"Block #9 (9a) - LIB #8 (8a)",
		},
		{
			"upsert, first insert, second update, undo initial insert",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "to", "receiver3"),
				),
				blockUndo(t, "9a", finalBlock("8a")),
			),
			nil,
			"Block #9 (9a) - LIB #8 (8a)",
		},
		{
			"upsert, first insert, second update, undo last update",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "to", "receiver3"),
				),
				blockUndo(t, "10a", finalBlock("8a")),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "12", Number: "34", From: "sender2", To: "receiver2"},
			}),
			"Block #10 (10a) - LIB #8 (8a)",
		},
		{
			"upsert, first insert, second update, third update, undo last update",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "to", "receiver3"),
				),
				dbChangesBlockData(t, "12a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "from", "sender3"),
				),
				blockUndo(t, "11a", finalBlock("8a")),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "12", Number: "34", From: "sender2", To: "receiver3"},
			}),
			"Block #11 (11a) - LIB #8 (8a)",
		},

		// Delete testing

		{
			"insert then delete - composite primary key",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					insertRowCompositePK("xfer", pk("id", "12", "number", "34"), "from", "sender1", "to", "receiver1"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					deleteRowMultiplePK("xfer", pk("id", "12", "number", "34")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				require.Empty(t, readDbChangesRows[XferCompositePKRow](t, dbx, schema, "xfer"))
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"insert, update, then delete - composite primary key",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					insertRowCompositePK("xfer", pk("id", "12", "number", "34"), "from", "sender1", "to", "receiver1"),
				),
				dbChangesBlockData(t, "11a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("id", "12", "number", "34"), "to", "receiver2"),
				),
				dbChangesBlockData(t, "12a", finalBlock("12a"),
					deleteRowMultiplePK("xfer", pk("id", "12", "number", "34")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				require.Empty(t, readDbChangesRows[XferCompositePKRow](t, dbx, schema, "xfer"))
			},
			"Block #12 (12a) - LIB #12 (12a)",
		},
		{
			"multiple inserts with different composite keys, delete one",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					insertRowCompositePK("xfer", pk("id", "12", "number", "34"), "from", "sender1", "to", "receiver1"),
					insertRowCompositePK("xfer", pk("id", "56", "number", "78"), "from", "sender2", "to", "receiver2"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					deleteRowMultiplePK("xfer", pk("id", "12", "number", "34")),
				),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "56", Number: "78", From: "sender2", To: "receiver2"},
			}),
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"insert then delete with undo",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					insertRowCompositePK("xfer", pk("id", "12", "number", "34"), "from", "sender1", "to", "receiver1"),
				),
				dbChangesBlockData(t, "11a", finalBlock("8a"),
					deleteRowMultiplePK("xfer", pk("id", "12", "number", "34")),
				),
				blockUndo(t, "10a", finalBlock("8a")),
			),
			equalsXferCompositePKRows([]*XferCompositePKRow{
				{ID: "12", Number: "34", From: "sender1", To: "receiver1"},
			}),
			"Block #10 (10a) - LIB #8 (8a)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
				nil,
				nil,
				tablesInput(func(schema string) map[string]*db2.TableInfo {
					return db2.TestTables(schema, map[string]*db2.TableInfo{
						"xfer": mustNewTableInfo(schema, "xfer", []string{"id", "number"}, map[string]*db2.ColumnInfo{
							"id":     db2.NewColumnInfo("id", "text", ""),
							"number": db2.NewColumnInfo("number", "bigint", ""),
							"from":   db2.NewColumnInfo("from", "text", ""),
							"to":     db2.NewColumnInfo("to", "text", ""),
						}),
					})
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func TestSinker_Integration_CompositePrimaryKey_CamelCase(t *testing.T) {
	pk := compositePK

	tests := []sinkerTestCase{
		{
			"upsert final with camelCase composite primary key",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowMultiplePK("xfer", pk("userAddress", "0x123", "tokenId", "456"), "from", "sender1", "to", "receiver1"),
				),
			),
			equalsXferCamelCasePKRows([]*XferCamelCasePKRow{
				{UserAddress: "0x123", TokenId: "456", From: "sender1", To: "receiver1"},
			}),
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"upsert, first insert, second update, final with camelCase composite primary key",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("8a"),
					upsertRowMultiplePK("xfer", pk("userAddress", "0x123", "tokenId", "456"), "from", "sender1", "to", "receiver1"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowMultiplePK("xfer", pk("userAddress", "0x123", "tokenId", "456"), "to", "receiver2"),
				),
			),
			equalsXferCamelCasePKRows([]*XferCamelCasePKRow{
				{UserAddress: "0x123", TokenId: "456", From: "sender1", To: "receiver2"},
			}),
			"Block #11 (11a) - LIB #11 (11a)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
				nil,
				nil,
				tablesInput(func(schema string) map[string]*db2.TableInfo {
					return db2.TestTables(schema, map[string]*db2.TableInfo{
						"xfer": mustNewTableInfo(schema, "xfer", []string{"userAddress", "tokenId"}, map[string]*db2.ColumnInfo{
							"userAddress": db2.NewColumnInfo("userAddress", "text", ""),
							"tokenId":     db2.NewColumnInfo("tokenId", "text", ""),
							"from":        db2.NewColumnInfo("from", "text", ""),
							"to":          db2.NewColumnInfo("to", "text", ""),
						}),
					})
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func TestSinker_Integration_Bytes(t *testing.T) {
	type XferBytesRow struct {
		ID    []byte `db:"id"`
		Value []byte `db:"value"`
	}

	runSinkerTest(
		t,
		sharedDbChangesPostgresContainer,
		nil,
		nil,
		tablesInput(func(schema string) map[string]*db2.TableInfo {
			return db2.TestTables(schema, map[string]*db2.TableInfo{
				"xfer": mustNewTableInfo(schema, "xfer", []string{"id"}, map[string]*db2.ColumnInfo{
					"id":    db2.NewColumnInfo("id", "bytea", []byte{}),
					"value": db2.NewColumnInfo("value", "bytea", []byte{}),
				}),
			})
		}),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowSinglePK("xfer", `\x01`, "value", `\x04ab`),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			require.Equal(t,
				[]*XferBytesRow{{ID: []byte{0x01}, Value: []byte{0x04, 0xab}}},
				readDbChangesRows[XferBytesRow](t, dbx, schema, "xfer"),
			)
		},
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func TestSinker_Integration_TimescaleDB(t *testing.T) {
	container, cleanup := setupRawPostgresContainer(PostgresContainerConfig{Image: "timescale/timescaledb:latest-pg16"})
	t.Cleanup(cleanup)

	pk := compositePK

	runSinkerTest(
		t,
		container,
		nil,
		nil,
		rawSQLInput(func(schema string) string {
			// Note: tsdb.segmentby and tsdb.orderby options automatically enable columnstore
			// in TimescaleDB 2.19+, so no explicit add_columnstore_policy call is needed.
			return `
				CREATE TABLE IF NOT EXISTS trades (
					id BIGSERIAL,
					block_time TIMESTAMPTZ NOT NULL,
					token_id BYTEA NOT NULL,
					PRIMARY KEY (id, block_time)
				) WITH (
					tsdb.hypertable,
					tsdb.partition_column='block_time',
					tsdb.segmentby='token_id',
					tsdb.orderby='block_time DESC'
				);
			`
		}),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowCompositePK("trades", pk("id", "1", "block_time", "2023-10-01T00:00:00Z"), "token_id", `\x04ab`),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			type Row struct {
				ID        []byte    `db:"id"`
				BlockTime time.Time `db:"block_time"`
				TokenID   []byte    `db:"token_id"`
			}

			require.Equal(t, []*Row{
				{ID: []byte{0x31}, BlockTime: blockTime(t, "2023-10-01T00:00:00Z"), TokenID: []byte{0x04, 0xab}},
			}, readRowsBy[Row](t, dbx, fmt.Sprintf(`"%s"."trades"`, schema), "id, block_time"))
		},
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func TestSinker_Integration_ParentChildOrdering(t *testing.T) {
	runSinkerTest(
		t,
		sharedDbChangesPostgresContainer,
		nil,
		nil,
		rawSQLInput(func(schema string) string {
			return fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS "%s".users (
					id TEXT PRIMARY KEY
				);
				CREATE TABLE IF NOT EXISTS "%s".xfer (
					id TEXT PRIMARY KEY,
					"from" TEXT,
					CONSTRAINT fk_users
						FOREIGN KEY("from")
						REFERENCES "%s".users(id)
				);
			`, schema, schema, schema)
		},
		),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowSinglePK("users", "user1"),
				insertRowSinglePK("xfer", "xfer1", "from", "user1"),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			type XferRow struct {
				ID   string `db:"id"`
				From string `db:"from"`
			}

			type UserRow struct {
				ID string `db:"id"`
			}

			require.Equal(t,
				[]*XferRow{{ID: "xfer1", From: "user1"}},
				readDbChangesRows[XferRow](t, dbx, schema, "xfer"),
			)
		},
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func TestSinker_Integration_BatchOrdinalSimple(t *testing.T) {
	// Custom sinker factory options to force batching across blocks
	customizeFactoryOptions := func(defaults sinker.SinkerFactoryOptions) sinker.SinkerFactoryOptions {
		defaults.BatchBlockFlushInterval = 10
		defaults.BatchRowFlushInterval = 10
		return defaults
	}

	runSinkerTest(
		t,
		sharedDbChangesPostgresContainer,
		nil,
		customizeFactoryOptions,
		rawSQLInput(func(schema string) string {
			return fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS "%s".orders (
					id TEXT PRIMARY KEY,
					amount TEXT
				);
			`, schema)
		}),
		streamMock(
			// Block 10a: Create order1
			dbChangesBlockData(t, "10a", finalBlock("8a"),
				insertRowSinglePK("orders", "order1", "amount", "100"),
			),
			// Block 11a: Create order2
			dbChangesBlockData(t, "11a", finalBlock("8a"),
				insertRowSinglePK("orders", "order2", "amount", "200"),
			),
			// Block 12a: Create order3 and trigger flush
			dbChangesBlockData(t, "12a", finalBlock("12a"),
				insertRowSinglePK("orders", "order3", "amount", "300"),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			type OrderRow struct {
				ID     string `db:"id"`
				Amount string `db:"amount"`
			}

			rows := readDbChangesRows[OrderRow](t, dbx, schema, "orders")
			require.Len(t, rows, 3)

			// All orders should exist regardless of ordinal system
			require.Contains(t, rows, &OrderRow{ID: "order1", Amount: "100"})
			require.Contains(t, rows, &OrderRow{ID: "order2", Amount: "200"})
			require.Contains(t, rows, &OrderRow{ID: "order3", Amount: "300"})
		},
		"Block #12 (12a) - LIB #12 (12a)",
	)
}

func TestSinker_Integration_ParentChildOrderingBatched(t *testing.T) {
	// Custom options - don't change sink options
	customizeOptions := func(defaults []sink.Option) []sink.Option {
		return defaults
	}

	// Custom sinker factory options to force batching across blocks
	customizeFactoryOptions := func(defaults sinker.SinkerFactoryOptions) sinker.SinkerFactoryOptions {
		// Set BatchBlockFlushInterval to 10 to ensure all blocks are batched together
		// Also increase BatchRowFlushInterval to prevent early row-based flushing
		defaults.BatchBlockFlushInterval = 10
		defaults.BatchRowFlushInterval = 10
		return defaults
	}

	runSinkerTest(
		t,
		sharedDbChangesPostgresContainer,
		customizeOptions,
		customizeFactoryOptions,
		rawSQLInput(func(schema string) string {
			return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS "%s".users (
				id TEXT PRIMARY KEY
			);
			CREATE TABLE IF NOT EXISTS "%s".xfer (
				id TEXT PRIMARY KEY,
				"from" TEXT,
				CONSTRAINT fk_users
					FOREIGN KEY("from")
					REFERENCES "%s".users(id)
			);
		`, schema, schema, schema)
		}),
		streamMock(
			// Block 10a: Create user1 first (batch ordinal 0)
			dbChangesBlockData(t, "10a", finalBlock("8a"),
				insertRowSinglePK("users", "user1"),
			),
			// Block 11a: Create xfer1 referencing user1 (batch ordinal 1)
			// With batch-tied ordinals, this gets ordinal 1, ensuring proper ordering
			dbChangesBlockData(t, "11a", finalBlock("8a"),
				insertRowSinglePK("xfer", "xfer1", "from", "user1"),
			),
			// Block 12a: Trigger the flush by reaching final block
			dbChangesBlockData(t, "12a", finalBlock("12a"),
				insertRowSinglePK("users", "user2"),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			type XferRow struct {
				ID   string `db:"id"`
				From string `db:"from"`
			}

			type UserRow struct {
				ID string `db:"id"`
			}

			// Both rows should exist - this test might fail with incorrect ordinal ordering
			// because user1 must be inserted before xfer1 due to foreign key constraint
			// With block-local ordinals, both get ordinal 0, causing potential ordering issues
			require.Equal(t,
				[]*UserRow{{ID: "user1"}, {ID: "user2"}},
				readDbChangesRows[UserRow](t, dbx, schema, "users"),
			)

			require.Equal(t,
				[]*XferRow{{ID: "xfer1", From: "user1"}},
				readDbChangesRows[XferRow](t, dbx, schema, "xfer"),
			)
		},
		"Block #12 (12a) - LIB #12 (12a)",
	)
}

func TestSinker_Integration_ComplexDependentTableOrdering(t *testing.T) {
	runSinkerTest(
		t,
		sharedDbChangesPostgresContainer,
		nil,
		nil,
		rawSQLInput(func(schema string) string {
			return fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS "%[1]s".departments (
					id TEXT PRIMARY KEY
				);
				CREATE TABLE IF NOT EXISTS "%[1]s".employees (
					id TEXT PRIMARY KEY,
					department_id TEXT NOT NULL,
					CONSTRAINT fk_department
						FOREIGN KEY(department_id)
						REFERENCES "%[1]s".departments(id)
				);

				-- Pre-existing data, like if the sinker had stopped at that point
				INSERT INTO "%[1]s".departments (id) VALUES ('dept1');
			`, schema)
		}),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("10a"),
				insertRowSinglePK("employees", "emp1", "department_id", "dept1"),
				insertRowSinglePK("departments", "dept2"),
				insertRowSinglePK("employees", "emp2", "department_id", "dept2"),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			type DepartmentRow struct {
				ID string `db:"id"`
			}

			type EmployeeRow struct {
				ID           string `db:"id"`
				DepartmentID string `db:"department_id"`
			}

			require.Equal(t, []*DepartmentRow{
				{ID: "dept1"},
				{ID: "dept2"},
			}, readDbChangesRows[DepartmentRow](t, dbx, schema, "departments"))

			require.Equal(t, []*EmployeeRow{
				{ID: "emp1", DepartmentID: "dept1"},
				{ID: "emp2", DepartmentID: "dept2"},
			}, readDbChangesRows[EmployeeRow](t, dbx, schema, "employees"))
		},
		"Block #10 (10a) - LIB #10 (10a)",
	)
}

func TestSinker_Integration_UndoBufferWorks(t *testing.T) {
	runSinkerTest(
		t,
		sharedDbChangesPostgresContainer,
		func(defaults []sink.Option) []sink.Option {
			return append(defaults, sink.WithBlockDataBuffer(2))
		},
		nil,
		tablesInput(func(schema string) map[string]*db2.TableInfo { return db2.TestSinglePrimaryKeyTables(schema) }),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("8a"),
				insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1"),
			),
			dbChangesBlockData(t, "11a", finalBlock("8a"),
				insertRowSinglePK("xfer", "5678", "from", "sender2", "to", "receiver2"),
			),
			dbChangesBlockData(t, "12a", finalBlock("8a"),
				insertRowSinglePK("xfer", "9101", "from", "sender3", "to", "receiver3"),
			),
		),
		func(t *testing.T, dbx *sqlx.DB, schema string) {
			require.Equal(t, []*XferSinglePKRow{
				{ID: "1234", From: "sender1", To: "receiver1"},
			}, readDbChangesRows[XferSinglePKRow](t, dbx, schema, "xfer"))
		},
		"Block #10 (10a) - LIB #8 (8a)",
	)
}

func TestSinker_Integration_DeltaUpdate_Add(t *testing.T) {
	type CounterRow struct {
		ID    string `db:"id"`
		Count int64  `db:"count"`
	}

	tests := []sinkerTestCase{
		{
			"add - initial upsert sets the value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"add - multiple adds accumulate within same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("100")),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"add - adds accumulate across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("25")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 125}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"add - negative delta subtracts",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("-30")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 70}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"add - insert then update with delta",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", deltaAdd("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"add - insert with delta sets initial value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", deltaAdd("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"add - insert with delta then update with delta accumulates",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", deltaAdd("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", deltaAdd("25")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 125}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"add - multiple updates with delta in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
					updateRowSinglePK("counters", "counter1", "count", deltaAdd("20")),
					updateRowSinglePK("counters", "counter1", "count", deltaAdd("30")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"add - set then add in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("10")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 110}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"add - set then add across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("10")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", "50"),
					upsertRowSinglePK("counters", "counter1", "count", deltaAdd("10")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 60}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"sub - basic subtraction",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaSub("30")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 70}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"sub - multiple subtractions accumulate",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaSub("20")),
					upsertRowSinglePK("counters", "counter1", "count", deltaSub("15")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 65}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"sub - subtracting negative value adds",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaSub("-25")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 125}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
				nil,
				nil,
				rawSQLInput(func(schema string) string {
					return fmt.Sprintf(`
						CREATE TABLE IF NOT EXISTS "%s".counters (
							id TEXT PRIMARY KEY,
							count BIGINT DEFAULT 0
						);
					`, schema)
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func TestSinker_Integration_DeltaUpdate_Max(t *testing.T) {
	type CounterRow struct {
		ID    string `db:"id"`
		Count int64  `db:"count"`
	}

	tests := []sinkerTestCase{
		{
			"max - initial upsert sets the value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"max - keeps higher value in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("100")),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"max - updates to higher value in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("50")),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"max - keeps higher value across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"max - updates to higher value across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("50")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"max - insert then update with max",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", deltaMax("150")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"max - insert then update with lower max keeps original",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", deltaMax("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"max - insert with max sets initial value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", deltaMax("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"max - multiple updates with max in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
					updateRowSinglePK("counters", "counter1", "count", deltaMax("80")),
					updateRowSinglePK("counters", "counter1", "count", deltaMax("150")),
					updateRowSinglePK("counters", "counter1", "count", deltaMax("120")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"max - set then max in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("150")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"max - set then max across blocks overwrites",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("150")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					// set(50) resets the value, then max(80) computes max(50,80)=80
					// Since UpdateOp stays SET after set→max, the flush overwrites with 80
					upsertRowSinglePK("counters", "counter1", "count", "50"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("80")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 80}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"max - handles negative values",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("-50")),
					upsertRowSinglePK("counters", "counter1", "count", deltaMax("-100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: -50}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
				nil,
				nil,
				rawSQLInput(func(schema string) string {
					return fmt.Sprintf(`
						CREATE TABLE IF NOT EXISTS "%s".counters (
							id TEXT PRIMARY KEY,
							count BIGINT DEFAULT 0
						);
					`, schema)
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func TestSinker_Integration_DeltaUpdate_Min(t *testing.T) {
	type CounterRow struct {
		ID    string `db:"id"`
		Count int64  `db:"count"`
	}

	tests := []sinkerTestCase{
		{
			"min - initial upsert sets the value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"min - keeps lower value in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("50")),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"min - updates to lower value in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("100")),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"min - keeps lower value across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("50")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"min - updates to lower value across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"min - insert then update with min",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", deltaMin("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"min - insert then update with higher min keeps original",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", deltaMin("150")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"min - insert with min sets initial value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", deltaMin("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"min - multiple updates with min in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
					updateRowSinglePK("counters", "counter1", "count", deltaMin("120")),
					updateRowSinglePK("counters", "counter1", "count", deltaMin("50")),
					updateRowSinglePK("counters", "counter1", "count", deltaMin("80")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"min - set then min in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 50}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"min - set then min across blocks overwrites",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("50")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					// set(200) resets the value, then min(150) computes min(200,150)=150
					// Since UpdateOp stays SET after set→min, the flush overwrites with 150
					upsertRowSinglePK("counters", "counter1", "count", "200"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("150")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 150}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"min - handles negative values",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("-50")),
					upsertRowSinglePK("counters", "counter1", "count", deltaMin("-100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: -100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
				nil,
				nil,
				rawSQLInput(func(schema string) string {
					return fmt.Sprintf(`
						CREATE TABLE IF NOT EXISTS "%s".counters (
							id TEXT PRIMARY KEY,
							count BIGINT DEFAULT 0
						);
					`, schema)
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func TestSinker_Integration_DeltaUpdate_SetIfNull(t *testing.T) {
	type CounterRow struct {
		ID    string `db:"id"`
		Count int64  `db:"count"`
	}

	tests := []sinkerTestCase{
		{
			"set_if_null - initial upsert sets the value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"set_if_null - keeps first value in same block",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"set_if_null - keeps first value across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"set_if_null - insert then update with set_if_null keeps original",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					updateRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"set_if_null - insert with set_if_null sets initial value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					insertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"set_if_null - multiple set_if_null in same block keeps first",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("300")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"set_if_null - set then set_if_null in same block keeps set value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"set_if_null - set then set_if_null across blocks keeps set value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"set_if_null - set after set_if_null in same block overwrites",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					// SET_IF_NULL → SET is a valid transition in the same block
					// The SET value overwrites the SET_IF_NULL value
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
					upsertRowSinglePK("counters", "counter1", "count", "200"),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 200}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		{
			"set_if_null - set overwrites previous set_if_null across blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", "200"),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 200}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"set_if_null - handles negative values",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("-100")),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("-50")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: -100}, rows[0])
			},
			"Block #10 (10a) - LIB #10 (10a)",
		},
		// Mixed set_if_null and set across blocks
		{
			"set_if_null - set in block1 then set_if_null in block2 keeps block1 value",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"set_if_null - set_if_null in block1 then set in block2 overwrites",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", "200"),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 200}, rows[0])
			},
			"Block #11 (11a) - LIB #11 (11a)",
		},
		{
			"set_if_null - alternating set and set_if_null across three blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", "200"),
				),
				dbChangesBlockData(t, "12a", finalBlock("12a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("300")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				// Block1: set_if_null(100) -> 100
				// Block2: set(200) -> 200 (overwrites)
				// Block3: set_if_null(300) -> 200 (keeps existing)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 200}, rows[0])
			},
			"Block #12 (12a) - LIB #12 (12a)",
		},
		{
			"set_if_null - set then set_if_null then set across three blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", "100"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
				dbChangesBlockData(t, "12a", finalBlock("12a"),
					upsertRowSinglePK("counters", "counter1", "count", "300"),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				// Block1: set(100) -> 100
				// Block2: set_if_null(200) -> 100 (keeps existing)
				// Block3: set(300) -> 300 (overwrites)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 300}, rows[0])
			},
			"Block #12 (12a) - LIB #12 (12a)",
		},
		{
			"set_if_null - multiple set_if_null across blocks only first wins",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("100")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("200")),
				),
				dbChangesBlockData(t, "12a", finalBlock("12a"),
					upsertRowSinglePK("counters", "counter1", "count", setIfNull("300")),
				),
			),
			func(t *testing.T, dbx *sqlx.DB, schema string) {
				rows := readDbChangesRows[CounterRow](t, dbx, schema, "counters")
				require.Len(t, rows, 1)
				require.Equal(t, &CounterRow{ID: "counter1", Count: 100}, rows[0])
			},
			"Block #12 (12a) - LIB #12 (12a)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSinkerTest(
				t,
				sharedDbChangesPostgresContainer,
				nil,
				nil,
				rawSQLInput(func(schema string) string {
					return fmt.Sprintf(`
						CREATE TABLE IF NOT EXISTS "%s".counters (
							id TEXT PRIMARY KEY,
							count BIGINT DEFAULT 0
						);
					`, schema)
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

type sinkerTestCase struct {
	name                string
	responses           []any
	expected            func(t *testing.T, dbx *sqlx.DB, schema string)
	expectedFinalCursor string
}

func runSinkerTest(
	t *testing.T,
	postgresContainer *PostgresContainerExt,
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

	schemaName := randomSchemaName()
	dsnRaw := postgresContainer.ConnectionString + "&schemaName=" + schemaName

	substreamsClientConfig := setupFakeSubstreamsServer(t, responses...)
	spkg := substreamsTestPackage(pbdatabase.File_sf_substreams_sink_database_v1_database_proto, (*pbdatabase.DatabaseChanges)(nil).ProtoReflect().Descriptor())

	var err error
	spkg.SinkConfig, err = anypb.New(&pbsql.Service{
		Schema: sqlPreambule(schemaName) + setupInput.ToSQL(schemaName),
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
	require.NoError(t, err)

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
		manifest.ModuleHash{},
		substreamsClientConfig,
		logger,
		tracer,
		baseSinkOptions...,
	)
	require.NoError(t, err)

	// Create sinker factory options
	options := sinker.SinkerFactoryOptions{
		CursorTableName:         setupOptions.CursorTableName,
		HistoryTableName:        setupOptions.HistoryTableName,
		ClickhouseCluster:       setupOptions.ClickhouseCluster,
		BatchBlockFlushInterval: 1,
		BatchRowFlushInterval:   5,
		LiveBlockFlushInterval:  1,
		OnModuleHashMismatch:    setupOptions.OnModuleHashMismatch,
		HandleReorgs:            true,
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

	db, err := sqlx.Connect("postgres", postgresContainer.ConnectionString)
	require.NoError(t, err)
	defer db.Close()

	if expected != nil {
		expected(t, db, schemaName)
	}

	// Fetch cursor directly from database instead of using GetCursor
	var cursorStr string
	cursorQuery := fmt.Sprintf(`SELECT cursor FROM "%s"."cursors" WHERE id = $1`, schemaName)
	err = db.GetContext(ctx, &cursorStr, cursorQuery, dbSinker.OutputModuleHash())
	require.NoError(t, err)

	finalCursor, err := bstream.CursorFromOpaque(cursorStr)
	require.NoError(t, err)

	actualCursor := fmt.Sprintf("Block %s - LIB %s", finalCursor.Block, finalCursor.LIB)
	require.Equal(t, expectedFinalCursor, actualCursor)
}

func randomSchemaName() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}
	return "testschema" + string(b)
}

// deltaValue wraps a value with an UpdateOp for delta updates
type deltaValue struct {
	value    string
	updateOp pbdatabase.Field_UpdateOp
}

func deltaAdd(value string) deltaValue { return deltaValue{value, pbdatabase.Field_UPDATE_OP_ADD} }
func deltaSub(value string) deltaValue {
	// Negate the value for subtraction (sub is just add with negated value)
	if len(value) > 0 && value[0] == '-' {
		return deltaValue{value[1:], pbdatabase.Field_UPDATE_OP_ADD} // Remove the minus sign
	}
	return deltaValue{"-" + value, pbdatabase.Field_UPDATE_OP_ADD} // Add minus sign
}
func deltaMax(value string) deltaValue { return deltaValue{value, pbdatabase.Field_UPDATE_OP_MAX} }
func deltaMin(value string) deltaValue { return deltaValue{value, pbdatabase.Field_UPDATE_OP_MIN} }
func setIfNull(value string) deltaValue {
	return deltaValue{value, pbdatabase.Field_UPDATE_OP_SET_IF_NULL}
}

func getFields(fieldsAndValues ...any) (out []*pbdatabase.Field) {
	if len(fieldsAndValues)%2 != 0 {
		panic("getFields needs even number of fieldsAndValues")
	}
	for i := 0; i < len(fieldsAndValues); i += 2 {
		name, ok := fieldsAndValues[i].(string)
		if !ok {
			panic(fmt.Sprintf("field name at index %d must be a string, got %T", i, fieldsAndValues[i]))
		}

		field := &pbdatabase.Field{Name: name}
		switch v := fieldsAndValues[i+1].(type) {
		case string:
			field.Value = v
		case deltaValue:
			field.Value = v.value
			field.UpdateOp = v.updateOp
		default:
			panic(fmt.Sprintf("field value at index %d must be string or deltaValue, got %T", i+1, fieldsAndValues[i+1]))
		}
		out = append(out, field)
	}
	return
}

func compositePK(keyValuePairs ...string) map[string]string {
	if len(keyValuePairs)%2 != 0 {
		panic("compositePK needs even number of keyValuePairs")
	}
	out := make(map[string]string)
	for i := 0; i < len(keyValuePairs); i += 2 {
		out[keyValuePairs[i]] = keyValuePairs[i+1]
	}
	return out
}

func insertRowSinglePK(table string, pk string, fieldsAndValues ...any) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_Pk{
			Pk: pk,
		},
		Operation: pbdatabase.TableChange_OPERATION_CREATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func insertRowCompositePK(table string, pk map[string]string, fieldsAndValues ...any) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_CREATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func updateRowSinglePK(table string, pk string, fieldsAndValues ...any) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_Pk{
			Pk: pk,
		},
		Operation: pbdatabase.TableChange_OPERATION_UPDATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func upsertRowSinglePK(table string, pk string, fieldsAndValues ...any) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_Pk{
			Pk: pk,
		},
		Operation: pbdatabase.TableChange_OPERATION_UPSERT,
		Fields:    getFields(fieldsAndValues...),
	}
}

func upsertRowMultiplePK(table string, pk map[string]string, fieldsAndValues ...any) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_UPSERT,
		Fields:    getFields(fieldsAndValues...),
	}
}

func updateRowMultiplePK(table string, pk map[string]string, fieldsAndValues ...any) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_UPDATE,
		Fields:    getFields(fieldsAndValues...),
	}
}
func deleteRowMultiplePK(table string, pk map[string]string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_DELETE,
	}
}

func dbChangesBlockData(t *testing.T, blockIdentifier string, lib finalBlock, changes ...*pbdatabase.TableChange) *pbsubstreamsrpc.Response {
	t.Helper()

	return blockScopedData(t, blockIdentifier, &pbdatabase.DatabaseChanges{TableChanges: changes}, lib)
}

func mustNewTableInfo(schema, name string, pkList []string, columnsByName map[string]*db2.ColumnInfo) *db2.TableInfo {
	ti, err := db2.NewTableInfo(schema, name, pkList, columnsByName)
	if err != nil {
		panic(err)
	}
	return ti
}

type sinkerSetupInput interface {
	ToSQL(schema string) string
}

func tablesInput(createTables func(schema string) map[string]*db2.TableInfo) tablesInputType {
	return tablesInputType{generator: createTables}
}

type tablesInputType struct {
	generator func(schema string) map[string]*db2.TableInfo
}

func (t tablesInputType) ToSQL(schema string) string {
	return db2.GenerateCreateTableSQL(t.generator(schema))
}

func rawSQLInput(generator func(schema string) string) rawSQLInputType {
	return rawSQLInputType{generator: generator}
}

type rawSQLInputType struct {
	generator func(schema string) string
}

func (r rawSQLInputType) ToSQL(schema string) string {
	return cli.Dedent(r.generator(schema))
}

func sqlPreambule(schema string) string {
	return fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s";
SET search_path TO "%s", public;`+"\n\n", schema, schema)
}

type XferSinglePKRow struct {
	ID   string `db:"id"`
	From string `db:"from"`
	To   string `db:"to"`
}

func equalsXferRows(expected []*XferSinglePKRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		require.Equal(t, expected, readDbChangesRows[XferSinglePKRow](t, dbx, schema, "xfer"))
	}
}

type XferCompositePKRow struct {
	ID     string `db:"id"`
	Number string `db:"number"`
	From   string `db:"from"`
	To     string `db:"to"`
}

func equalsXferCompositePKRows(expected []*XferCompositePKRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		require.Equal(t, expected, readDbChangesRows[XferCompositePKRow](t, dbx, schema, "xfer"))
	}
}

type XferCamelCasePKRow struct {
	UserAddress string `db:"userAddress"`
	TokenId     string `db:"tokenId"`
	From        string `db:"from"`
	To          string `db:"to"`
}

func equalsXferCamelCasePKRows(expected []*XferCamelCasePKRow) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		// Use custom ordering since this table doesn't have an "id" column
		actual := readRowsBy[XferCamelCasePKRow](t, dbx, fmt.Sprintf(`"%s"."%s"`, schema, "xfer"), `"userAddress", "tokenId"`)
		require.Equal(t, expected, actual)
	}
}
