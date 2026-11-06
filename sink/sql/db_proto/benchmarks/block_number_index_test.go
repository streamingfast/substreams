package benchmarks

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestBlockNumberIndexCost measures what the index on _block_number_ costs and what it
// buys, at a size where the answer is not noise.
//
// The sink creates it on every table because every table carries that column and every
// reorg deletes from every table by it — a foreign key indexes its referenced side only.
// The question that decides whether it should be on by default is what the load pays for
// it, and that has two shapes: built after the load, which is when the constraint pass
// runs, or already in place while the rows arrive, which is --apply-constraints=always.
//
// The row is shaped like erc20-balance-changes' map_balance_changes output: an id, two
// addresses, two balances as text, a transaction hash and an ordinal, plus the two columns
// the sink adds to every table.
//
// PGBENCH_TARGET_BYTES sets the heap size to aim for, 10GiB by default. Each variant drops
// what came before it, so the peak on disk is one table plus its index rather than all of
// them at once.
func TestBlockNumberIndexCost(t *testing.T) {
	requireBenchmark(t)

	ctx := context.Background()

	targetBytes := int64(envInt(t, "PGBENCH_TARGET_BYTES", 10*1024*1024*1024))
	blockSpan := envInt(t, "PGBENCH_BLOCKS", 500_000)

	// Measured at ~328 bytes of heap per row for this shape, tuple header and alignment
	// included; the table size actually reached is reported rather than assumed.
	rowCount := targetBytes / 328

	pool, dsn := startBenchmarkPostgres(t, ctx)
	_ = dsn

	t.Logf("target %s, %d rows over %d blocks", humanBytes(targetBytes), rowCount, blockSpan)

	type result struct {
		name       string
		load       time.Duration
		indexBuild time.Duration
		tableBytes int64
		indexBytes int64
		undo       time.Duration
		plan       string
	}
	var results []result

	// Variant one: load bare, then build the index, which is what the sink does.
	{
		createBalanceSchema(t, ctx, pool)

		load := copyBalanceRows(t, ctx, pool, rowCount, blockSpan)
		tableBytes := relationBytes(t, ctx, pool, "idx.balance_changes")

		undoBare, planBare := timeUndo(t, ctx, pool, blockSpan)

		startAt := time.Now()
		_, err := pool.Exec(ctx, `CREATE INDEX balance_changes_block_number_idx ON idx.balance_changes (_block_number_)`)
		require.NoError(t, err)
		indexBuild := time.Since(startAt)

		undoIndexed, planIndexed := timeUndo(t, ctx, pool, blockSpan)

		results = append(results,
			result{name: "load bare, index after (the default)", load: load, indexBuild: indexBuild,
				tableBytes: tableBytes, indexBytes: relationBytes(t, ctx, pool, "balance_changes_block_number_idx"), undo: undoIndexed, plan: planIndexed},
			result{name: "  same data, undo without the index", undo: undoBare, plan: planBare},
		)
	}

	// Variant two: the index already in place while the rows arrive.
	{
		createBalanceSchema(t, ctx, pool)
		_, err := pool.Exec(ctx, `CREATE INDEX balance_changes_block_number_idx ON idx.balance_changes (_block_number_)`)
		require.NoError(t, err)

		load := copyBalanceRows(t, ctx, pool, rowCount, blockSpan)
		undo, plan := timeUndo(t, ctx, pool, blockSpan)

		results = append(results, result{
			name:       "index in place during the load",
			load:       load,
			tableBytes: relationBytes(t, ctx, pool, "idx.balance_changes"),
			indexBytes: relationBytes(t, ctx, pool, "balance_changes_block_number_idx"),
			undo:       undo,
			plan:       plan,
		})
	}

	fmt.Printf("\n%-38s %10s %12s %10s %10s %12s  %s\n", "variant", "load", "index build", "table", "index", "undo 1k blk", "plan")
	for _, r := range results {
		fmt.Printf("%-38s %10s %12s %10s %10s %12s  %s\n",
			r.name,
			durationOrDash(r.load),
			durationOrDash(r.indexBuild),
			bytesOrDash(r.tableBytes),
			bytesOrDash(r.indexBytes),
			durationOrDash(r.undo),
			r.plan)
	}
	fmt.Println()
}

func createBalanceSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS idx CASCADE; CREATE SCHEMA idx`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE idx.balance_changes (
			_block_number_    INTEGER NOT NULL,
			_block_timestamp_ TIMESTAMP NOT NULL,
			id                TEXT NOT NULL,
			contract          TEXT NOT NULL,
			owner             TEXT NOT NULL,
			old_balance       TEXT NOT NULL,
			new_balance       TEXT NOT NULL,
			transaction_id    TEXT NOT NULL,
			ordinal           BIGINT NOT NULL
		)`)
	require.NoError(t, err)
}

// copyBalanceRows streams the rows straight into COPY rather than materialising them: at
// this size the slice alone would not fit in memory.
func copyBalanceRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rowCount int64, blockSpan int) time.Duration {
	t.Helper()

	source := &balanceRowSource{
		remaining: rowCount,
		total:     rowCount,
		blockSpan: int64(blockSpan),
		random:    rand.New(rand.NewSource(1)),
		baseTime:  time.Unix(1_600_000_000, 0).UTC(),
	}

	startAt := time.Now()
	copied, err := pool.CopyFrom(ctx,
		pgx.Identifier{"idx", "balance_changes"},
		[]string{"_block_number_", "_block_timestamp_", "id", "contract", "owner", "old_balance", "new_balance", "transaction_id", "ordinal"},
		source)
	require.NoError(t, err)
	require.Equal(t, rowCount, copied)

	return time.Since(startAt)
}

// balanceRowSource generates the rows lazily, one at a time.
type balanceRowSource struct {
	remaining int64
	total     int64
	blockSpan int64
	random    *rand.Rand
	baseTime  time.Time
	current   []any
}

func (s *balanceRowSource) Next() bool {
	if s.remaining == 0 {
		return false
	}
	s.remaining--

	// Blocks in order, as a backfill produces them, so the heap is clustered by block the
	// way a real load leaves it.
	index := s.total - s.remaining - 1
	blockNumber := index * s.blockSpan / s.total

	s.current = []any{
		int32(blockNumber),
		s.baseTime.Add(time.Duration(blockNumber) * 12 * time.Second),
		hexOf(s.random, 32),
		hexOf(s.random, 20),
		hexOf(s.random, 20),
		strconv.FormatUint(s.random.Uint64(), 10),
		strconv.FormatUint(s.random.Uint64(), 10),
		hexOf(s.random, 32),
		int64(index % 64),
	}

	return true
}

func (s *balanceRowSource) Values() ([]any, error) { return s.current, nil }
func (s *balanceRowSource) Err() error             { return nil }

const hexDigits = "0123456789abcdef"

func hexOf(random *rand.Rand, bytes int) string {
	out := make([]byte, 2+bytes*2)
	out[0], out[1] = '0', 'x'
	for i := 2; i < len(out); i++ {
		out[i] = hexDigits[random.Intn(16)]
	}

	return string(out)
}

// timeUndo measures the reorg path: the sink deletes from every table by _block_number_,
// so this is what one table costs. It rolls back, the point being the scan rather than the
// write, and the next measurement needing the same rows.
//
// ANALYZE first, or the planner is choosing without statistics — a COPY leaves none behind
// and autovacuum has not necessarily caught up, which is enough to make it seq scan a
// predicate that matches a fraction of a percent. The plan is reported alongside the
// duration so a number that looks wrong can be explained rather than guessed at.
//
// Twice, reporting the second: the first pass pays for whatever the load evicted, and a
// variant measured cold against another measured warm compares the cache, not the index.
func timeUndo(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blockSpan int) (time.Duration, string) {
	t.Helper()

	_, err := pool.Exec(ctx, `ANALYZE idx.balance_changes`)
	require.NoError(t, err)

	var duration time.Duration
	for range 2 {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)

		startAt := time.Now()
		_, err = tx.Exec(ctx, `DELETE FROM idx.balance_changes WHERE _block_number_ > $1`, blockSpan-1_000)
		require.NoError(t, err)
		duration = time.Since(startAt)

		require.NoError(t, tx.Rollback(ctx))
	}

	return duration, undoPlan(t, ctx, pool, blockSpan)
}

// undoPlan names the scan the planner chose for that delete.
func undoPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blockSpan int) string {
	t.Helper()

	rows, err := pool.Query(ctx, `EXPLAIN DELETE FROM idx.balance_changes WHERE _block_number_ > $1`, blockSpan-1_000)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))

		trimmed := strings.TrimSpace(line)
		for _, node := range []string{"Seq Scan", "Index Scan", "Bitmap Heap Scan", "Bitmap Index Scan"} {
			if strings.HasPrefix(trimmed, "->  "+node) || strings.HasPrefix(trimmed, node) {
				return node
			}
		}
	}

	return "?"
}

func relationBytes(t *testing.T, ctx context.Context, pool *pgxpool.Pool, relation string) int64 {
	t.Helper()

	qualified := relation
	if !containsDot(relation) {
		qualified = "idx." + relation
	}

	var size int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT pg_relation_size($1)`, qualified).Scan(&size))

	return size
}

func containsDot(s string) bool {
	for i := range s {
		if s[i] == '.' {
			return true
		}
	}

	return false
}

func startBenchmarkPostgres(t *testing.T, ctx context.Context) (*pgxpool.Pool, string) {
	t.Helper()

	container, err := tcpostgres.Run(ctx, envString("PGBENCH_PG_IMAGE", "postgres:17-alpine"),
		tcpostgres.WithDatabase("bench"),
		tcpostgres.WithUsername("bench"),
		tcpostgres.WithPassword("bench"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(120*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	dsn := container.MustConnectionString(ctx, "sslmode=disable")
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// An index build is bounded by maintenance_work_mem, and the default 64MB says more
	// about the default than about the index.
	_, err = pool.Exec(ctx, `ALTER SYSTEM SET maintenance_work_mem = '1GB'`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `SELECT pg_reload_conf()`)
	require.NoError(t, err)

	return pool, dsn
}

func durationOrDash(d time.Duration) string {
	if d == 0 {
		return "-"
	}

	return d.Round(time.Millisecond).String()
}

func bytesOrDash(n int64) string {
	if n == 0 {
		return "-"
	}

	return humanBytes(n)
}
