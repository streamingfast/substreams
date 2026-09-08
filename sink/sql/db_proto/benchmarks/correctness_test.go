package benchmarks

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPgCopyBinaryRoundTrip is the safety net for the binary encoder: a value written
// through pgcopy must come back out of the server byte-identical to what a
// parameterised INSERT of the same value produces. Binary COPY does no coercion, so an
// encoder bug shows up as either a hard COPY failure or, worse, silently wrong data.
func TestPgCopyBinaryRoundTrip(t *testing.T) {

	ctx := context.Background()
	pool := startPostgres(t, ctx)

	_, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+benchSchema)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, createBenchTableSQL)
	require.NoError(t, err)

	columns, err := pgcopy.LoadColumns(ctx, pool, benchSchema, benchTable)
	require.NoError(t, err)

	rows := generateRows(1000, 7)

	// Reference load through parameterised INSERT, which does coerce.
	_, err = pool.Exec(ctx, "TRUNCATE bench.transfers")
	require.NoError(t, err)
	statement := insertPlaceholderSQL()
	for i, r := range rows {
		values := r.values()
		require.NoError(t, pgcopy.NormalizeRow(columns, values))
		_, err := pool.Exec(ctx, statement, values...)
		require.NoError(t, err, "row %d", i)
	}
	viaInsert := dumpRows(t, ctx, pool)

	// Same dataset through the binary COPY path.
	_, err = pool.Exec(ctx, "TRUNCATE bench.transfers")
	require.NoError(t, err)

	path := t.TempDir() + "/roundtrip.pgcopy"
	file, err := os.Create(path)
	require.NoError(t, err)
	writer, err := pgcopy.NewWriter(file, columns)
	require.NoError(t, err)
	for i, r := range rows {
		values := r.values()
		require.NoError(t, pgcopy.NormalizeRow(columns, values))
		require.NoError(t, writer.WriteRow(values), "row %d", i)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())

	reader, err := os.Open(path)
	require.NoError(t, err)
	defer reader.Close()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	tag, err := conn.Conn().PgConn().CopyFrom(ctx, reader, pgcopy.CopySQL(benchSchema, benchTable, columns))
	require.NoError(t, err)
	require.Equal(t, int64(len(rows)), tag.RowsAffected())

	viaCopy := dumpRows(t, ctx, pool)

	require.Equal(t, len(viaInsert), len(viaCopy))
	for i := range viaInsert {
		require.Equal(t, viaInsert[i], viaCopy[i], "row %d differs between INSERT and binary COPY", i)
	}
}

// dumpRows reads every column back in a stable order, as Go values, so two load paths
// can be compared field by field.
func dumpRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()

	const query = `
		SELECT _block_number_, _block_timestamp_, id, encode(tx_hash, 'hex'), log_index,
		       "from", "to", amount::text, gas_used::text, success, fee, topics, meta::text
		FROM bench.transfers
		ORDER BY id, log_index`

	rows, err := pool.Query(ctx, query)
	require.NoError(t, err)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var (
			blockNumber int64
			timestamp   time.Time
			id, hash    string
			logIndex    int32
			from, to    string
			amount, gas string
			success     bool
			fee         float64
			topics      []string
			meta        string
		)
		require.NoError(t, rows.Scan(&blockNumber, &timestamp, &id, &hash, &logIndex,
			&from, &to, &amount, &gas, &success, &fee, &topics, &meta))

		out = append(out, fmt.Sprintf("%d|%s|%s|%s|%d|%s|%s|%s|%s|%t|%v|%v|%s",
			blockNumber, timestamp.UTC().Format(time.RFC3339Nano), id, hash, logIndex,
			from, to, amount, gas, success, fee, topics, meta))
	}
	require.NoError(t, rows.Err())

	return out
}

func startPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	container, err := tcpostgres.Run(ctx, envString("PGBENCH_PG_IMAGE", "postgres:17-alpine"),
		tcpostgres.WithDatabase("bench"),
		tcpostgres.WithUsername("bench"),
		tcpostgres.WithPassword("bench"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	pool, err := pgxpool.New(ctx, container.MustConnectionString(ctx, "sslmode=disable"))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}
