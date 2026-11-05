package db

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresFlush_ErrorNamesTheFailedStatement(t *testing.T) {
	ctx := context.Background()
	schema := pgTestSchema(t)
	l := pgTestLoader(t, schema, true)
	require.NoError(t, l.InsertCursor(ctx, testModuleHash, pgCursor(1, 1)))

	require.NoError(t, l.Insert("acc", map[string]string{"id": "taken"}, map[string]FieldData{"n": {Value: "1"}}, nil))
	_, err := l.Flush(ctx, testModuleHash, pgCursor(2, 2), 2)
	require.NoError(t, err)

	countRows := func() (count int) {
		require.NoError(t, l.QueryRowContext(ctx, fmt.Sprintf(`SELECT count(*) FROM %s.acc`, EscapeIdentifier(l.dsn.Schema()))).Scan(&count))
		return count
	}
	for _, id := range []string{"taken", "bad"} {
		// A failed flush keeps its pending changes: each case starts from a new loader.
		l := pgTestLoader(t, schema, true)
		for i := range postgresBatchStatements + 10 {
			require.NoError(t, l.Insert("acc", map[string]string{"id": fmt.Sprintf("r%d", i)}, map[string]FieldData{"n": {Value: "1"}}, nil))
		}
		n := "2"
		if id == "bad" {
			n = "not-a-number"
		}
		require.NoError(t, l.Insert("acc", map[string]string{"id": id}, map[string]FieldData{"n": {Value: n}}, nil))

		_, err := l.Flush(ctx, testModuleHash, pgCursor(3, 3), 3)
		require.ErrorContains(t, err, "executing query")
		require.ErrorContains(t, err, fmt.Sprintf("'%s'", id), "the failing statement")
		assert.NotContains(t, err.Error(), "'r0'", "only the failing statement")
		assert.Equal(t, 1, countRows(), "nothing of the flush stays")
	}
}

// sendStatements gives how many statements completed before the error, whether the failed one
// returned rows or not.
func TestPostgresSendStatements_Completed(t *testing.T) {
	ctx := context.Background()
	l := pgTestLoader(t, pgTestSchema(t), true)
	for name, failing := range map[string]string{
		"no rows":         `SET statement_timeout = 'invalid'`,
		"after some rows": `SELECT 1 / (x - 1) FROM generate_series(0, 1) x`,
		"before any rows": `SELECT 1 / x FROM generate_series(0, 1) x`,
		"at parsing":      `NOT SQL`,
	} {
		t.Run(name, func(t *testing.T) {
			tx, err := l.Begin()
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			completed, err := tx.(statementSender).sendStatements(ctx, []string{`SELECT 1`, failing, `SELECT 2`})
			require.Error(t, err)
			assert.Equal(t, 1, completed)
		})
	}
}

// Each statement of a batch is its own for the server.
func TestPostgresStatementBatch_StatementsStaySeparate(t *testing.T) {
	ctx := context.Background()
	schema := pgTestSchema(t)
	l := pgTestLoader(t, schema, true)
	acc := EscapeIdentifier(l.dsn.Schema()) + ".acc"
	_, err := l.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN at timestamptz DEFAULT statement_timestamp()`, acc))
	require.NoError(t, err)

	tx, err := l.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	batch := postgresStatementBatch{tx: tx}
	require.NoError(t, batch.add(ctx, fmt.Sprintf(`INSERT INTO %s (id) VALUES ('a')`, acc)))
	require.NoError(t, batch.add(ctx, `SELECT pg_sleep(0.01)`))
	require.NoError(t, batch.add(ctx, fmt.Sprintf(`INSERT INTO %s (id) VALUES ('b')`, acc)))
	require.NoError(t, batch.flush(ctx))

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT count(DISTINCT at) FROM %s WHERE id IN ('a','b')`, acc))
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	var distinct int
	require.NoError(t, rows.Scan(&distinct))
	assert.Equal(t, 2, distinct, "statement_timestamp() differs between the statements of a batch")
}

// A cancelled flush stops its statement on the server and leaves the connection usable.
func TestPostgresStatementBatch_Cancel(t *testing.T) {
	l := pgTestLoader(t, pgTestSchema(t), true)
	ctx, cancel := context.WithCancel(context.Background())
	tx, err := l.BeginTx(ctx, nil)
	require.NoError(t, err)
	time.AfterFunc(100*time.Millisecond, cancel)

	batch := postgresStatementBatch{tx: tx}
	require.NoError(t, batch.add(ctx, "SELECT 1"))
	require.NoError(t, batch.add(ctx, "SELECT pg_sleep(30)"))
	start := time.Now()
	err = batch.flush(ctx)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.True(t, IsCancellation(err), "%v", err)
	_ = tx.Rollback()

	var running int
	require.NoError(t, l.QueryRowContext(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE query = 'SELECT pg_sleep(30)' AND state = 'active'`).Scan(&running))
	assert.Zero(t, running)
}

func TestPostgresStatementBatch_Limits(t *testing.T) {
	ctx := context.Background()
	tx := &recordingSender{}
	batch := postgresStatementBatch{tx: tx}
	for i := range postgresBatchStatements + 5 {
		require.NoError(t, batch.add(ctx, fmt.Sprintf("SELECT %d", i)))
	}
	require.NoError(t, batch.flush(ctx))
	require.Len(t, tx.sent, 2)
	assert.Len(t, tx.sent[0], postgresBatchStatements)
	assert.Equal(t, "SELECT 0", tx.sent[0][0], "in order")
	assert.Len(t, tx.sent[1], 5)

	tx = &recordingSender{}
	batch = postgresStatementBatch{tx: tx}
	require.NoError(t, batch.add(ctx, strings.Repeat("x", postgresBatchBytes-10)))
	require.NoError(t, batch.add(ctx, strings.Repeat("y", 20)))
	require.Len(t, tx.sent, 1, "sent before going over the size limit")
	assert.Len(t, tx.sent[0], 1)
	require.NoError(t, batch.add(ctx, strings.Repeat("z", 2*postgresBatchBytes)))
	require.NoError(t, batch.flush(ctx))
	assert.Len(t, tx.sent, 3, "a statement over the limit goes alone")
}

type recordingSender struct {
	TestTx
	sent [][]string
}

func (t *recordingSender) sendStatements(ctx context.Context, statements []string) (int, error) {
	t.sent = append(t.sent, slices.Clone(statements))
	return len(statements), nil
}

// BenchmarkPostgresFlush times a flush against the database in PG_DSN.
func BenchmarkPostgresFlush(b *testing.B) {
	const rows = 1000
	ctx := context.Background()
	l := pgTestLoader(b, pgTestSchema(b), true)
	require.NoError(b, l.InsertCursor(ctx, testModuleHash, pgCursor(1, 1)))

	block := uint64(1)
	for b.Loop() {
		b.StopTimer()
		block++
		for i := range rows {
			id := map[string]string{"id": fmt.Sprintf("%d-%d", block, i)}
			require.NoError(b, l.Insert("acc", id, map[string]FieldData{"n": {Value: "1"}}, nil))
		}
		b.StartTimer()
		_, err := l.Flush(ctx, testModuleHash, pgCursor(block, block), block)
		require.NoError(b, err)
	}
	b.ReportMetric(float64(rows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
}
