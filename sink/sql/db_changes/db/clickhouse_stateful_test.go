package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const statefulTestSchema = `
	CREATE TABLE balances (
		id String,
		amount UInt256,
		n Nullable(Int64),
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY id SETTINGS index_granularity = 256;
	CREATE TABLE transfers (
		id String,
		amount UInt256,
		_version_ UInt64,
		_block_num_ UInt64
	) ENGINE = MergeTree ORDER BY id;
	CREATE TABLE trades (
		id String,
		amount Int64,
		_version_ UInt64,
		_block_num_ UInt64,
		_sign_ Int8
	) ENGINE = CollapsingMergeTree(_sign_) ORDER BY id;
	CREATE TABLE uids (
		id UUID,
		c Int64,
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY id;
	CREATE TABLE trades_nb (
		id String,
		amount Int64,
		_version_ UInt64,
		_sign_ Int8
	) ENGINE = CollapsingMergeTree(_sign_) ORDER BY id;
	CREATE TABLE trades_total (k String, total Int64) ENGINE = SummingMergeTree ORDER BY k;
	CREATE MATERIALIZED VIEW trades_total_mv TO trades_total AS SELECT 'all' AS k, sum(amount * _sign_) AS total FROM trades GROUP BY k;
	CREATE TABLE ticks (
		ts DateTime,
		at DateTime64(3),
		price Int64,
		ratio Nullable(Float32),
		fee Decimal(18, 4),
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY ts;
	CREATE TABLE counters (
		n UInt64,
		c Int64,
		label String DEFAULT 'none',
		score Int32 DEFAULT 7,
		tag LowCardinality(Nullable(String)),
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY n;
	CREATE TABLE codes (
		code FixedString(4),
		c Int64,
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY code;
	CREATE TABLE typed (
		id String,
		m Map(String, String),
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY id;
	CREATE TABLE events (
		at DateTime64(3),
		c Int64,
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY at;
`

// newStatefulTestLoader sets up a fresh ClickHouse database (CH_DSN, without database) in stateful
// mode, or opens it again when database is given.
func newStatefulTestLoader(t *testing.T, database string) (*Loader, string) {
	return newStatefulTestLoaderWithReorgs(t, database, false)
}

func newStatefulTestLoaderWithReorgs(t *testing.T, database string, reorgs bool) (*Loader, string) {
	t.Helper()
	base := os.Getenv("CH_DSN")
	if base == "" {
		t.Skip(`CH_DSN not set, please specify CH_DSN to run this test, example: CH_DSN="clickhouse://default:password@localhost:9000"`)
	}
	ctx := context.Background()

	fresh := database == ""
	if fresh {
		database = fmt.Sprintf("stateful_%d", time.Now().UnixNano())
		admin, err := ParseDSN(base + "/default")
		require.NoError(t, err)
		adminLoader, err := NewLoader(admin, testCursorTableName, testHistoryTableName, "", 0, 0, 0, OnModuleHashMismatchIgnore.String(), new(bool), zlog, tracer)
		require.NoError(t, err)
		_, err = adminLoader.ExecContext(ctx, "CREATE DATABASE "+database)
		require.NoError(t, err)
		require.NoError(t, adminLoader.Close())
		t.Cleanup(func() {
			if os.Getenv("KEEP_DB") != "" {
				return
			}
			if l, err := NewLoader(admin, testCursorTableName, testHistoryTableName, "", 0, 0, 0, OnModuleHashMismatchIgnore.String(), new(bool), zlog, tracer); err == nil {
				_, _ = l.ExecContext(ctx, "DROP DATABASE IF EXISTS "+database)
				_ = l.Close()
			}
		})
	}

	dsn, err := ParseDSN(base + "/" + database)
	require.NoError(t, err)
	l, err := NewLoader(dsn, testCursorTableName, testHistoryTableName, "", 0, 0, 0, OnModuleHashMismatchIgnore.String(), new(bool), zlog, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	if fresh {
		// Setup creates the history table whether the run handles reorgs or not.
		require.NoError(t, l.EnableClickhouseStateful(DefaultJournalTableName, testHistoryTableName, true))
		require.NoError(t, l.Setup(ctx, database, statefulTestSchema, false))
	}
	require.NoError(t, l.EnableClickhouseStateful(DefaultJournalTableName, testHistoryTableName, reorgs))
	require.NoError(t, l.LoadTables(database, testCursorTableName, testHistoryTableName))
	if fresh {
		require.NoError(t, l.InsertCursor(ctx, testModuleHash, pgCursor(1, 1)))
	}
	require.NoError(t, l.PrepareClickhouseStateful(ctx))
	require.NoError(t, l.RecoverClickhouseStateful(ctx))

	cursor, rawCursor, _, err := l.GetCursor(ctx, testModuleHash)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	l.SetExpectedCursor(rawCursor)
	return l, database
}

func balances(t *testing.T, l *Loader) map[string]string {
	t.Helper()
	rows, err := l.Query("SELECT id, toString(amount), ifNull(toString(n), 'NULL') FROM balances FINAL WHERE _deleted_ = 0 ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, amount, n string
		require.NoError(t, rows.Scan(&id, &amount, &n))
		out[id] = amount + "/" + n
	}
	require.NoError(t, rows.Err())
	return out
}

func transfers(t *testing.T, l *Loader) []string {
	t.Helper()
	rows, err := l.Query("SELECT id FROM transfers ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		out = append(out, id)
	}
	require.NoError(t, rows.Err())
	return out
}

func flushStatefulTest(t *testing.T, l *Loader, block uint64) error {
	t.Helper()
	_, err := l.Flush(context.Background(), testModuleHash, pgCursor(block, block), block)
	return err
}

func TestClickhouseStateful_Operations(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "b"}, map[string]FieldData{"amount": {Value: "1"}, "n": {Value: "3"}}, nil))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t1"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))
	assert.Equal(t, map[string]string{"a": "10/NULL", "b": "1/3"}, balances(t, l))

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}, "n": {Value: "-2", UpdateOp: UpdateOpMax}}, nil))
	require.NoError(t, l.Update("balances", map[string]string{"id": "missing"}, map[string]FieldData{"amount": {Value: "1"}}, nil))
	require.NoError(t, l.Delete("balances", map[string]string{"id": "b"}, nil))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t2"}, map[string]FieldData{"amount": {Value: "5"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))
	assert.Equal(t, map[string]string{"a": "15/0"}, balances(t, l))
	assert.Equal(t, []string{"t1", "t2"}, transfers(t, l))

	// A deleted key can come back.
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "b"}, map[string]FieldData{"amount": {Value: "7", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 4))
	assert.Equal(t, map[string]string{"a": "15/0", "b": "7/NULL"}, balances(t, l))

	// Mutations on an append table are refused.
	require.NoError(t, l.Upsert("transfers", map[string]string{"id": "t1"}, map[string]FieldData{"amount": {Value: "1"}}, nil))
	err := flushStatefulTest(t, l, 5)
	require.ErrorContains(t, err, "only accepts inserts")
}

func TestClickhouseStateful_FlushRetryAfterPartialWrite(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t1"}, map[string]FieldData{"amount": {Value: "5"}}, nil))

	statefulFlushTestHook = func() error { return errors.New("lost before the cursor") }
	require.ErrorContains(t, flushStatefulTest(t, l, 3), "lost before the cursor")
	statefulFlushTestHook = nil

	// The sinker retries the flush with the same pending changes: applied once, not twice.
	require.NoError(t, flushStatefulTest(t, l, 3))
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, l))
	assert.Equal(t, []string{"t1"}, transfers(t, l))
}

func TestClickhouseStateful_RecoveryOnStartup(t *testing.T) {
	l, database := newStatefulTestLoader(t, "")

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "new"}, map[string]FieldData{"amount": {Value: "1"}}, nil))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t1"}, map[string]FieldData{"amount": {Value: "5"}}, nil))
	statefulFlushTestHook = func() error { return errors.New("crash") }
	require.Error(t, flushStatefulTest(t, l, 3))
	statefulFlushTestHook = nil
	assert.Equal(t, map[string]string{"a": "15/NULL", "new": "1/NULL"}, balances(t, l), "the crashed flush wrote its rows")

	// The process dies; a new one rolls the flush back before streaming from the stored cursor.
	restarted, _ := newStatefulTestLoader(t, database)
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, restarted))
	assert.Empty(t, transfers(t, restarted))

	// The stream sends those blocks again.
	require.NoError(t, restarted.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, restarted.Insert("transfers", map[string]string{"id": "t1"}, map[string]FieldData{"amount": {Value: "5"}}, nil))
	require.NoError(t, flushStatefulTest(t, restarted, 3))
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, restarted))
	assert.Equal(t, []string{"t1"}, transfers(t, restarted))

	// Starting again finds nothing to roll back.
	again, _ := newStatefulTestLoader(t, database)
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, again))
}

func TestClickhouseStateful_ValuesRoundTrip(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")

	require.NoError(t, l.Upsert("ticks", map[string]string{"ts": "2024-01-02 03:04:05"}, map[string]FieldData{
		"at": {Value: "2024-01-02T03:04:05.123Z"}, "price": {Value: "1"}, "ratio": {Value: "0.5"},
	}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))

	// Same key written as Unix seconds: the same row. Untouched columns keep their exact values.
	require.NoError(t, l.Upsert("ticks", map[string]string{"ts": "1704164645"}, map[string]FieldData{"price": {Value: "2", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))

	var count uint64
	var at, ratio, fee string
	var price int64
	require.NoError(t, l.QueryRow("SELECT count(), any(toString(at)), any(price), any(toString(ratio)), any(toString(fee)) FROM ticks FINAL WHERE _deleted_ = 0").Scan(&count, &at, &price, &ratio, &fee))
	assert.Equal(t, uint64(1), count)
	assert.Equal(t, "2024-01-02 03:04:05.123", at)
	assert.Equal(t, int64(3), price)
	assert.Equal(t, "0.5", ratio)
	assert.Equal(t, "0", fee)
}

func TestClickhouseStateful_EquivalentKeysInOneFlush(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")

	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))

	require.NoError(t, l.Upsert("counters", map[string]string{"n": "01"}, map[string]FieldData{"c": {Value: "2", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "3", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))

	var count uint64
	var c int64
	require.NoError(t, l.QueryRow("SELECT count(), any(c) FROM counters FINAL WHERE _deleted_ = 0").Scan(&count, &c))
	assert.Equal(t, uint64(1), count)
	assert.Equal(t, int64(15), c)
}

func TestClickhouseStateful_RetryAfterCursorWrittenButReplyLost(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	d := l.dialect.(*ClickhouseDialect)

	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))

	var written *journalEntry
	statefulFlushTestHook = func() error { written = d.stateful.uncommitted; return nil }
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))
	statefulFlushTestHook = nil

	// The sink took that flush as failed (its reply was lost) and retries the same changes.
	d.stateful.uncommitted = written
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, l))
}

func TestClickhouseStateful_NewModuleHashContinuesVersions(t *testing.T) {
	l, database := newStatefulTestLoader(t, "")
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))

	// A new module version takes over the tables.
	other, _ := newStatefulTestLoader(t, database)
	ctx := context.Background()
	require.NoError(t, other.InsertCursor(ctx, "other", pgCursor(3, 3)))
	other.SetExpectedCursor(pgCursor(3, 3).String())
	require.NoError(t, other.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "100", UpdateOp: UpdateOpAdd}}, nil))
	_, err := other.Flush(ctx, "other", pgCursor(4, 4), 4)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "111/NULL"}, balances(t, other))
}

func TestClickhouseStateful_PrimaryKeyChangeRefused(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	require.NoError(t, l.Update("balances", map[string]string{"id": "a"}, map[string]FieldData{"id": {Value: "b"}}, nil))
	require.ErrorContains(t, flushStatefulTest(t, l, 2), "changing primary key column")
}

func TestTableRoleFor_BlockNumberNeededWithReorgs(t *testing.T) {
	table := mustNewTableInfo("s", "t", []string{"id"}, map[string]*ColumnInfo{"id": NewColumnInfo("id", "String", "")})
	_, err := tableRoleFor(table, "MergeTree", "MergeTree ORDER BY id", "id", map[string]string{statefulVersionColumn: "UInt64"}, true)
	require.ErrorContains(t, err, "add a _block_num_ UInt64 column")
}

func TestTableRoleFor(t *testing.T) {
	table := mustNewTableInfo("s", "t", []string{"id"}, map[string]*ColumnInfo{"id": NewColumnInfo("id", "String", "")})
	managed := map[string]string{statefulVersionColumn: "UInt64", statefulDeletedColumn: "UInt8"}

	tests := []struct {
		engine, engineFull, sortingKey string
		managed                        map[string]string
		role                           tableRole
		err                            string
	}{
		{"CollapsingMergeTree", "CollapsingMergeTree(_sign_) ORDER BY id", "id", map[string]string{statefulVersionColumn: "UInt64", statefulSignColumn: "Int8", statefulBlockNumColumn: "UInt64"}, tableRoleAppendSigned, ""},
		{"CollapsingMergeTree", "CollapsingMergeTree(sign) ORDER BY id", "id", map[string]string{statefulVersionColumn: "UInt64", statefulSignColumn: "Int8", statefulBlockNumColumn: "UInt64"}, 0, "CollapsingMergeTree(_sign_)"},
		{"ReplacingMergeTree", "ReplacingMergeTree(_version_, _deleted_) ORDER BY id SETTINGS index_granularity = 256", "id", managed, tableRoleKeyed, ""},
		{"ReplicatedReplacingMergeTree", "ReplicatedReplacingMergeTree('/clickhouse/tables/{uuid}/{shard}', '{replica}', _version_, _deleted_) ORDER BY id", "id", managed, tableRoleKeyed, ""},
		{"MergeTree", "MergeTree ORDER BY id", "id", map[string]string{statefulVersionColumn: "UInt64"}, tableRoleAppend, ""},
		{"ReplacingMergeTree", "ReplacingMergeTree ORDER BY id", "id", managed, 0, "ReplacingMergeTree(_version_, _deleted_)"},
		{"ReplacingMergeTree", "ReplacingMergeTree(_version_, _deleted_) ORDER BY (id, ts)", "id, ts", managed, 0, "must be the primary key"},
		{"MergeTree", "MergeTree ORDER BY id", "id", map[string]string{}, 0, "add a _version_ UInt64 column"},
		{"Log", "Log", "", managed, 0, "not a MergeTree"},
		{"SummingMergeTree", "SummingMergeTree ORDER BY id", "id", managed, 0, "merges rows"},
	}
	for _, test := range tests {
		t.Run(test.engineFull, func(t *testing.T) {
			role, err := tableRoleFor(table, test.engine, test.engineFull, test.sortingKey, test.managed, false)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.role, role)
		})
	}
}

func tradesState(t *testing.T, l *Loader) (map[string]int64, int64) {
	t.Helper()
	rows, err := l.Query("SELECT id, sum(amount * _sign_) FROM trades GROUP BY id HAVING sum(_sign_) > 0 ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var amount int64
		require.NoError(t, rows.Scan(&id, &amount))
		out[id] = amount
	}
	require.NoError(t, rows.Err())
	var total int64
	require.NoError(t, l.QueryRow("SELECT sum(total) FROM trades_total").Scan(&total))
	return out, total
}

func revertStatefulTest(t *testing.T, l *Loader, lastValid uint64, cursorBlock, lib uint64) {
	t.Helper()
	require.NoError(t, l.Revert(context.Background(), testModuleHash, pgCursor(cursorBlock, lib), lastValid))
}

func TestClickhouseStateful_Revert(t *testing.T) {
	l, database := newStatefulTestLoaderWithReorgs(t, "", true)
	at := func(block uint64) *uint64 { return &block }

	// Block 10, final.
	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t10"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, l.Insert("trades", map[string]string{"id": "x10"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))

	// Blocks 11 and 12, reversible, in one flush.
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, at(11)))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t11"}, map[string]FieldData{"amount": {Value: "5"}}, at(11)))
	require.NoError(t, l.Insert("trades", map[string]string{"id": "x11"}, map[string]FieldData{"amount": {Value: "5"}}, at(11)))
	l.SetCurrentBlock(12)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "100", UpdateOp: UpdateOpAdd}}, at(12)))
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "b"}, map[string]FieldData{"amount": {Value: "1"}}, at(12)))
	require.NoError(t, l.Delete("balances", map[string]string{"id": "a"}, at(12)))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t12"}, map[string]FieldData{"amount": {Value: "100"}}, at(12)))
	require.NoError(t, l.Insert("trades", map[string]string{"id": "x12"}, map[string]FieldData{"amount": {Value: "100"}}, at(12)))
	_, err := l.Flush(context.Background(), testModuleHash, pgCursor(12, 10), 10)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"b": "1/NULL"}, balances(t, l))
	trades, total := tradesState(t, l)
	assert.Equal(t, map[string]int64{"x10": 10, "x11": 5, "x12": 100}, trades)
	assert.Equal(t, int64(115), total)

	// Block 12 is forked out.
	revertStatefulTest(t, l, 11, 11, 10)
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, l))
	assert.Equal(t, []string{"t10", "t11"}, transfers(t, l))
	trades, total = tradesState(t, l)
	assert.Equal(t, map[string]int64{"x10": 10, "x11": 5}, trades)
	assert.Equal(t, int64(15), total, "the materialized view follows the cancel rows")

	// The same undo again (sent again after a crash) changes nothing.
	revertStatefulTest(t, l, 11, 11, 10)
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, l))
	trades, total = tradesState(t, l)
	assert.Equal(t, map[string]int64{"x10": 10, "x11": 5}, trades)
	assert.Equal(t, int64(15), total)

	// Then block 11 too, from a restarted sink.
	restarted, _ := newStatefulTestLoaderWithReorgs(t, database, true)
	require.NoError(t, restarted.Revert(context.Background(), testModuleHash, pgCursor(10, 10), 10))
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, restarted))
	assert.Equal(t, []string{"t10"}, transfers(t, restarted))
	trades, total = tradesState(t, restarted)
	assert.Equal(t, map[string]int64{"x10": 10}, trades)
	assert.Equal(t, int64(10), total)

	// A new fork writes on top.
	restarted.SetCurrentBlock(11)
	require.NoError(t, restarted.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, at(11)))
	_, err = restarted.Flush(context.Background(), testModuleHash, pgCursor(11, 10), 10)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "11/NULL"}, balances(t, restarted))
}

func TestClickhouseStateful_RollbackKeepsMaterializedViewsRight(t *testing.T) {
	l, database := newStatefulTestLoader(t, "")

	require.NoError(t, l.Insert("trades", map[string]string{"id": "x1"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))

	require.NoError(t, l.Insert("trades", map[string]string{"id": "x2"}, map[string]FieldData{"amount": {Value: "5"}}, nil))
	statefulFlushTestHook = func() error { return errors.New("crash") }
	require.Error(t, flushStatefulTest(t, l, 3))
	statefulFlushTestHook = nil

	// Restarted twice: rolled back twice, still right.
	newStatefulTestLoader(t, database)
	restarted, _ := newStatefulTestLoader(t, database)
	trades, total := tradesState(t, restarted)
	assert.Equal(t, map[string]int64{"x1": 10}, trades)
	assert.Equal(t, int64(10), total)
}

func TestClickhouseStateful_RecoveryWaitsForRecover(t *testing.T) {
	l, database := newStatefulTestLoader(t, "")
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
	statefulFlushTestHook = func() error { return errors.New("crash") }
	require.Error(t, flushStatefulTest(t, l, 3))
	statefulFlushTestHook = nil

	// A standby loads its tables while waiting for the lock: nothing is rolled back yet, the
	// active sink may still be committing.
	dsn, err := ParseDSN(os.Getenv("CH_DSN") + "/" + database)
	require.NoError(t, err)
	standby, err := NewLoader(dsn, testCursorTableName, testHistoryTableName, "", 0, 0, 0, OnModuleHashMismatchIgnore.String(), new(bool), zlog, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = standby.Close() })
	require.NoError(t, standby.EnableClickhouseStateful(DefaultJournalTableName, testHistoryTableName, false))
	require.NoError(t, standby.LoadTables(database, testCursorTableName, testHistoryTableName))
	require.NoError(t, standby.PrepareClickhouseStateful(context.Background()))
	assert.Equal(t, map[string]string{"a": "15/NULL"}, balances(t, standby))

	// Once it holds the lock.
	require.NoError(t, standby.RecoverClickhouseStateful(context.Background()))
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, standby))
}

func TestClickhouseStateful_JournalNotStoredIsNotPending(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	d := l.dialect.(*ClickhouseDialect)
	ctx := context.Background()
	seq := d.stateful.nextSeq

	_, err := l.ExecContext(ctx, "RENAME TABLE substreams_journal TO substreams_journal_away")
	require.NoError(t, err)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.Error(t, flushStatefulTest(t, l, 2))
	assert.Nil(t, d.stateful.uncommitted)
	assert.Equal(t, seq, d.stateful.nextSeq)

	_, err = l.ExecContext(ctx, "RENAME TABLE substreams_journal_away TO substreams_journal")
	require.NoError(t, err)
	require.NoError(t, flushStatefulTest(t, l, 2))
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, l))
}

func TestClickhouseStateful_InterruptedRevertCompletedOnStartup(t *testing.T) {
	l, database := newStatefulTestLoaderWithReorgs(t, "", true)
	d := l.dialect.(*ClickhouseDialect)
	ctx := context.Background()
	at := func(block uint64) *uint64 { return &block }

	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, at(11)))
	require.NoError(t, l.Insert("transfers", map[string]string{"id": "t11"}, map[string]FieldData{"amount": {Value: "1"}}, at(11)))
	_, err := l.Flush(ctx, testModuleHash, pgCursor(11, 10), 10)
	require.NoError(t, err)

	// The undo to 10 writes its rows, then the sink dies before its cursor.
	l.activeModuleHash, l.activeCursor = testModuleHash, pgCursor(10, 10)
	require.NoError(t, d.revertStateful(synchronousInserts(ctx), l, 10))
	stored, err := storedCursor(ctx, l, testModuleHash)
	require.NoError(t, err)
	assert.Equal(t, pgCursor(11, 10).String(), stored)

	// Restarting completes it, cursor included, whatever the stream would send next.
	restarted, _ := newStatefulTestLoaderWithReorgs(t, database, true)
	stored, err = storedCursor(ctx, restarted, testModuleHash)
	require.NoError(t, err)
	assert.Equal(t, pgCursor(10, 10).String(), stored)
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, restarted))
	assert.Empty(t, transfers(t, restarted))
}

func TestClickhouseStateful_RevertMatchesKeysWrittenDifferently(t *testing.T) {
	l, _ := newStatefulTestLoaderWithReorgs(t, "", true)
	at := func(block uint64) *uint64 { return &block }

	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "01"}, map[string]FieldData{"c": {Value: "15"}}, at(11)))
	_, err := l.Flush(context.Background(), testModuleHash, pgCursor(11, 10), 10)
	require.NoError(t, err)
	l.SetCurrentBlock(12)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "22"}}, at(12)))
	_, err = l.Flush(context.Background(), testModuleHash, pgCursor(12, 10), 10)
	require.NoError(t, err)

	require.NoError(t, l.Revert(context.Background(), testModuleHash, pgCursor(10, 10), 10))
	var count uint64
	var c int64
	require.NoError(t, l.QueryRow("SELECT count(), any(c) FROM counters FINAL WHERE _deleted_ = 0").Scan(&count, &c))
	assert.Equal(t, uint64(1), count)
	assert.Equal(t, int64(10), c)
}

func TestClickhouseStateful_RollbackSignedTableWithoutBlockNumber(t *testing.T) {
	l, database := newStatefulTestLoader(t, "")
	require.NoError(t, l.Insert("trades_nb", map[string]string{"id": "x1"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	statefulFlushTestHook = func() error { return errors.New("crash") }
	require.Error(t, flushStatefulTest(t, l, 2))
	statefulFlushTestHook = nil

	restarted, _ := newStatefulTestLoader(t, database)
	var live int64
	require.NoError(t, restarted.QueryRow("SELECT sum(_sign_) FROM trades_nb").Scan(&live))
	assert.Equal(t, int64(0), live)
}

func TestClickhouseStateful_NewRowTakesConstantDefaults(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "1"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"score": {Value: "3", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))

	var label string
	var score int32
	require.NoError(t, l.QueryRow("SELECT label, score FROM counters FINAL WHERE _deleted_ = 0").Scan(&label, &score))
	assert.Equal(t, "none", label)
	assert.Equal(t, int32(10), score)
}

func TestConstantDefault(t *testing.T) {
	for expression, expected := range map[string]string{"0": "0", "-1.5": "-1.5", "'0'": "0", "''": "", "false": "false"} {
		value, ok := constantDefault(expression)
		assert.True(t, ok, expression)
		assert.Equal(t, expected, value, expression)
	}
	for _, expression := range []string{"now()", "toDecimal64(0, 2)", "'it''s'"} {
		_, ok := constantDefault(expression)
		assert.False(t, ok, expression)
	}
}

func TestClickhouseStateful_UndoKeepingPendingChanges(t *testing.T) {
	l, database := newStatefulTestLoaderWithReorgs(t, "", true)
	ctx := context.Background()
	at := func(block uint64) *uint64 { return &block }

	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))

	// Blocks 11 and 12 pending, then an undo to 11: 11 is kept, written with the undo's cursor.
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, at(11)))
	l.SetCurrentBlock(12)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "100", UpdateOp: UpdateOpAdd}}, at(12)))

	// The sink dies once the kept changes are written, before the cursor.
	statefulFlushTestHook = func() error { return errors.New("crash") }
	require.Error(t, l.Revert(ctx, testModuleHash, pgCursor(11, 10), 11))
	statefulFlushTestHook = nil

	// Restarting goes back to the stored cursor: the stream sends block 11 again.
	restarted, _ := newStatefulTestLoaderWithReorgs(t, database, true)
	stored, err := storedCursor(ctx, restarted, testModuleHash)
	require.NoError(t, err)
	assert.Equal(t, pgCursor(10, 10).String(), stored)
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, restarted))

	// This time the undo goes through: block 11 written with its cursor, and a later restart
	// leaves it alone.
	restarted.SetCurrentBlock(11)
	require.NoError(t, restarted.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, at(11)))
	require.NoError(t, restarted.Revert(ctx, testModuleHash, pgCursor(11, 10), 11))
	again, _ := newStatefulTestLoaderWithReorgs(t, database, true)
	stored, err = storedCursor(ctx, again, testModuleHash)
	require.NoError(t, err)
	assert.Equal(t, pgCursor(11, 10).String(), stored)
	assert.Equal(t, map[string]string{"a": "11/NULL"}, balances(t, again))
}

func TestClickhouseStateful_PrimaryKeyChangedByDeltaRefused(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	require.NoError(t, l.Update("counters", map[string]string{"n": "1"}, map[string]FieldData{"n": {Value: "1", UpdateOp: UpdateOpAdd}}, nil))
	require.ErrorContains(t, flushStatefulTest(t, l, 2), "changing primary key column")
}

func TestClickhouseStateful_DefaultsApplyBeforeLaterBlocks(t *testing.T) {
	l, _ := newStatefulTestLoaderWithReorgs(t, "", true)
	at := func(block uint64) *uint64 { return &block }
	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "5"}, map[string]FieldData{"c": {Value: "1"}}, at(10)))
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "5"}, map[string]FieldData{"score": {Value: "3", UpdateOp: UpdateOpAdd}}, at(11)))
	_, err := l.Flush(context.Background(), testModuleHash, pgCursor(11, 9), 9)
	require.NoError(t, err)

	var score int32
	require.NoError(t, l.QueryRow("SELECT score FROM counters FINAL WHERE _deleted_ = 0 AND n = 5").Scan(&score))
	assert.Equal(t, int32(10), score, "7 (DEFAULT) + 3, as in separate flushes")
}

func TestClickhouseStateful_LowCardinalityNullKeptOnUpdate(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "1"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))
	require.NoError(t, l.Update("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "2"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))

	var isNull uint8
	require.NoError(t, l.QueryRow("SELECT isNull(tag) FROM counters FINAL WHERE _deleted_ = 0").Scan(&isNull))
	assert.Equal(t, uint8(1), isNull)
}

func TestClickhouseStateful_UUIDKeySpellings(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	require.NoError(t, l.Upsert("uids", map[string]string{"id": "A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11"}, map[string]FieldData{"c": {Value: "1"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 2))
	require.NoError(t, l.Upsert("uids", map[string]string{"id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"}, map[string]FieldData{"c": {Value: "1", UpdateOp: UpdateOpAdd}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 3))

	var count uint64
	var c int64
	require.NoError(t, l.QueryRow("SELECT count(), any(c) FROM uids FINAL WHERE _deleted_ = 0").Scan(&count, &c))
	assert.Equal(t, uint64(1), count)
	assert.Equal(t, int64(2), c)
}

func TestClickhouseStateful_RevertKeysWrittenDifferentlyInOneBlock(t *testing.T) {
	l, _ := newStatefulTestLoaderWithReorgs(t, "", true)
	at := func(block uint64) *uint64 { return &block }
	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "01"}, map[string]FieldData{"c": {Value: "5", UpdateOp: UpdateOpAdd}}, at(11)))
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "7", UpdateOp: UpdateOpAdd}}, at(11)))
	_, err := l.Flush(context.Background(), testModuleHash, pgCursor(11, 10), 10)
	require.NoError(t, err)

	require.NoError(t, l.Revert(context.Background(), testModuleHash, pgCursor(10, 10), 10))
	var c int64
	require.NoError(t, l.QueryRow("SELECT c FROM counters FINAL WHERE _deleted_ = 0").Scan(&c))
	assert.Equal(t, int64(10), c)
}

func TestClickhouseStateful_KeysStoredDifferently(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	// A FixedString key is stored padded, a DateTime64(3) key to the millisecond: the delta must
	// still find its row.
	for _, k := range []struct{ table, column, key string }{
		{"codes", "code", "ab"},
		{"events", "at", "2024-01-01T00:00:00.123456Z"},
	} {
		require.NoError(t, l.Upsert(k.table, map[string]string{k.column: k.key}, map[string]FieldData{"c": {Value: "10"}}, nil))
		require.NoError(t, flushStatefulTest(t, l, 2))
		require.NoError(t, l.Upsert(k.table, map[string]string{k.column: k.key}, map[string]FieldData{"c": {Value: "5", UpdateOp: UpdateOpAdd}}, nil))
		require.NoError(t, flushStatefulTest(t, l, 3))

		var count uint64
		var c int64
		require.NoError(t, l.QueryRow("SELECT count(), any(c) FROM "+k.table+" FINAL WHERE _deleted_ = 0").Scan(&count, &c))
		assert.Equal(t, uint64(1), count, k.table)
		assert.Equal(t, int64(15), c, k.table)
	}
}

func TestClickhouseStateful_UndoCutShortBeforeKeptChanges(t *testing.T) {
	l, database := newStatefulTestLoaderWithReorgs(t, "", true)
	d := l.dialect.(*ClickhouseDialect)
	ctx := context.Background()
	at := func(block uint64) *uint64 { return &block }

	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))

	// Blocks 11 and 12 pending, then an undo to 11: the sink dies once the revert is journaled,
	// before the kept changes of block 11 are.
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, at(11)))
	l.SetCurrentBlock(12)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "100", UpdateOp: UpdateOpAdd}}, at(12)))
	l.activeModuleHash, l.activeCursor = testModuleHash, pgCursor(11, 10)
	require.NoError(t, d.revertStateful(synchronousInserts(ctx), l, 11))

	// Restarting must not write the undo's cursor, which would skip block 11.
	restarted, _ := newStatefulTestLoaderWithReorgs(t, database, true)
	stored, err := storedCursor(ctx, restarted, testModuleHash)
	require.NoError(t, err)
	assert.Equal(t, pgCursor(10, 10).String(), stored)
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, restarted))
}

func TestClickhouseStateful_InterruptedRevertCompletedFromJournal(t *testing.T) {
	l, database := newStatefulTestLoaderWithReorgs(t, "", true)
	d := l.dialect.(*ClickhouseDialect)
	ctx := context.Background()
	at := func(block uint64) *uint64 { return &block }

	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "10"}}, nil))
	require.NoError(t, flushStatefulTest(t, l, 10))
	for block := uint64(11); block <= 12; block++ {
		l.SetCurrentBlock(block)
		require.NoError(t, l.Upsert("balances", map[string]string{"id": "a"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, at(block)))
		_, err := l.Flush(ctx, testModuleHash, pgCursor(block, 10), 10)
		require.NoError(t, err)
	}

	// The undo to 10 writes its rows, then dies with its history deletion done for block 11 only.
	l.activeModuleHash, l.activeCursor = testModuleHash, pgCursor(10, 10)
	require.NoError(t, d.revertStateful(synchronousInserts(ctx), l, 10))
	_, err := l.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (table_name, pk, block_num, prev_value, seq) VALUES (?, ?, 12, ?, 0)", d.historyTableIdentifier()),
		"balances", primaryKeyToJSON(map[string]string{"id": "a"}), `{"amount":"11","id":"a","n":null}`)
	require.NoError(t, err)

	// Completing it writes back what it read first, not the history left over.
	restarted, _ := newStatefulTestLoaderWithReorgs(t, database, true)
	assert.Equal(t, map[string]string{"a": "10/NULL"}, balances(t, restarted))
}

func TestClickhouseStateful_SetIfNullSameWhereverFlushesFall(t *testing.T) {
	l, _ := newStatefulTestLoaderWithReorgs(t, "", true)
	at := func(block uint64) *uint64 { return &block }
	ctx := context.Background()

	// Row 1 is created and filled in separate flushes, row 2 in one: c (not Nullable) is stored as
	// 0 when left out, so SET_IF_NULL leaves it in both.
	l.SetCurrentBlock(10)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"tag": {Value: "x"}}, at(10)))
	_, err := l.Flush(ctx, testModuleHash, pgCursor(10, 9), 9)
	require.NoError(t, err)
	l.SetCurrentBlock(11)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "1"}, map[string]FieldData{"c": {Value: "5", UpdateOp: UpdateOpSetIfNull}}, at(11)))
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "2"}, map[string]FieldData{"tag": {Value: "x"}}, at(11)))
	l.SetCurrentBlock(12)
	require.NoError(t, l.Upsert("counters", map[string]string{"n": "2"}, map[string]FieldData{"c": {Value: "5", UpdateOp: UpdateOpSetIfNull}}, at(12)))
	_, err = l.Flush(ctx, testModuleHash, pgCursor(12, 9), 9)
	require.NoError(t, err)

	rows, err := l.Query("SELECT n, c FROM counters FINAL WHERE _deleted_ = 0 ORDER BY n")
	require.NoError(t, err)
	defer rows.Close()
	got := map[uint64]int64{}
	for rows.Next() {
		var n uint64
		var c int64
		require.NoError(t, rows.Scan(&n, &c))
		got[n] = c
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[uint64]int64{1: 0, 2: 0}, got)
}

func TestClickhouseStateful_KeyedColumnNotConvertibleRefused(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	require.NoError(t, l.Upsert("typed", map[string]string{"id": "a"}, map[string]FieldData{}, nil))
	require.ErrorContains(t, flushStatefulTest(t, l, 2), `column "m" of type Map(String, String) cannot be read back`)
}

func TestClickhouseStateful_WritesStillRunningStoppedBeforeRollback(t *testing.T) {
	l, _ := newStatefulTestLoader(t, "")
	d := l.dialect.(*ClickhouseDialect)
	ctx := context.Background()

	// A write the sink gave up on, still running in ClickHouse (5 s, a row at a time).
	done := make(chan error, 1)
	go func() {
		_, err := l.ExecContext(d.sinkWrite(ctx), `INSERT INTO transfers (id, amount, _version_, _block_num_)
			SELECT toString(number), 1, 999, 1 FROM numbers(50) WHERE sleepEachRow(0.1) = 0 SETTINGS max_block_size = 1`)
		done <- err
	}()
	require.Eventually(t, func() bool {
		var running uint64
		require.NoError(t, l.QueryRow("SELECT count() FROM system.processes WHERE startsWith(query_id, ?)", d.sinkWritePrefix()).Scan(&running))
		return running > 0
	}, 5*time.Second, 50*time.Millisecond)

	require.NoError(t, d.stopSinkWrites(ctx, l))
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("the write is still running")
	}
	var running uint64
	require.NoError(t, l.QueryRow("SELECT count() FROM system.processes WHERE startsWith(query_id, ?)", d.sinkWritePrefix()).Scan(&running))
	assert.Zero(t, running)
}
