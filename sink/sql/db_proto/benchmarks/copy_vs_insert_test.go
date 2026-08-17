// Package benchmarks compares the strategies available to the from-proto SQL sink for
// getting rows into PostgreSQL, against a real server in a container.
//
// The question it answers is narrow on purpose: given rows that are already prepared,
// how much of the wall clock is the *strategy* rather than the data? Every artifact is
// materialised on disk before any timer starts, so nothing here measures row
// generation, protobuf decoding or SQL string building unless a variant's real
// implementation would do that work at flush time.
//
// Run it with:
//
//	go test ./sink/sql/db_proto/benchmarks/ -run TestCopyVsInsert -v -timeout 30m
//
// Environment:
//
//	PGBENCH_ROWS=250000        rows in the dataset
//	PGBENCH_REPEAT=1           passes over the variant set, best duration is reported
//	PGBENCH_WITH_INDEX=1       add a btree on id and on _block_number_ before loading
//	PGBENCH_PG_IMAGE=...       postgres image (default postgres:17-alpine)
//	PGBENCH_KEEP_DATA_DIR=...  reuse artifacts across runs instead of a temp dir
package benchmarks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// multiRowBatchSize is how many rows go into one INSERT statement. It matches the
// order of magnitude the sink produces today: blockBatchSize of 25 blocks times a few
// entities per block.
const multiRowBatchSize = 500

func TestCopyVsInsert(t *testing.T) {
	requireBenchmark(t)

	ctx := context.Background()
	rowCount := envInt(t, "PGBENCH_ROWS", 250_000)
	repeat := envInt(t, "PGBENCH_REPEAT", 1)

	h := newHarness(t, ctx)

	t.Logf("generating %d rows and materialising on-disk artifacts in %s", rowCount, h.dataDir)
	generateStart := time.Now()
	rows := generateRows(rowCount, 1)
	art := h.materialise(t, ctx, rows)
	t.Logf("artifacts ready in %s: binary=%s csv=%s multirow-sql=%s",
		time.Since(generateStart).Round(time.Millisecond),
		humanBytes(art.binaryBytes), humanBytes(art.csvBytes), humanBytes(art.multiRowBytes))

	expected := expectedChecksum(rows)

	variants := []variant{
		{
			name:  "insert-1row-prepared",
			notes: "per-row prepared INSERT in one tx (what RowInserter does)",
			bytes: art.multiRowBytes,
			run:   func(ctx context.Context) error { return h.insertPerRow(ctx, rows) },
		},
		{
			name:  "insert-multirow-built-at-flush",
			notes: "build the giant VALUES statement then exec (what AccumulatorInserter does)",
			bytes: art.multiRowBytes,
			run:   func(ctx context.Context) error { return h.insertMultiRowBuilt(ctx, rows) },
		},
		{
			name:  "insert-multirow-built-at-flush-libpq",
			notes: "same, through database/sql + lib/pq, to size the driver's share",
			bytes: art.multiRowBytes,
			run:   func(ctx context.Context) error { return h.insertMultiRowBuiltLibpq(ctx, rows) },
		},
		{
			name:  "insert-multirow-prebuilt-from-disk",
			notes: "statements already built on disk: isolates pure server-side cost",
			bytes: art.multiRowBytes,
			run:   func(ctx context.Context) error { return h.insertMultiRowPrebuilt(ctx, art.multiRowPath) },
		},
		{
			name:  "copy-csv-from-disk",
			notes: "COPY FROM STDIN (FORMAT CSV), io.Copy from file",
			bytes: art.csvBytes,
			run:   func(ctx context.Context) error { return h.copyFromFile(ctx, art.csvPath, h.csvCopySQL) },
		},
		{
			name:  "copy-binary-from-disk",
			notes: "COPY FROM STDIN (FORMAT BINARY), io.Copy from file -- the proposed spill path",
			bytes: art.binaryBytes,
			run:   func(ctx context.Context) error { return h.copyFromFile(ctx, art.binaryPath, h.binaryCopySQL) },
		},
		{
			name:  "copy-binary-encoded-at-flush",
			notes: "pgx.CopyFrom over in-memory rows: binary COPY without pre-encoding",
			bytes: art.binaryBytes,
			run:   func(ctx context.Context) error { return h.copyFromValues(ctx, rows) },
		},
	}

	best := map[string]time.Duration{}
	for pass := range repeat {
		for _, v := range variants {
			h.truncate(t, ctx)

			start := time.Now()
			err := v.run(ctx)
			elapsed := time.Since(start)
			require.NoError(t, err, "variant %q", v.name)

			actual := h.checksum(t, ctx)
			require.Equal(t, expected.String(), actual.String(),
				"variant %q loaded different data than the dataset", v.name)

			if prev, ok := best[v.name]; !ok || elapsed < prev {
				best[v.name] = elapsed
			}
			t.Logf("pass %d  %-36s %10s", pass+1, v.name, elapsed.Round(time.Millisecond))
		}
	}

	report(t, variants, best, rowCount)
}

type variant struct {
	name  string
	notes string
	bytes int64
	run   func(ctx context.Context) error
}

func report(t *testing.T, variants []variant, best map[string]time.Duration, rowCount int) {
	t.Helper()

	baseline := best["insert-multirow-built-at-flush"]

	out := &tabwriter.Writer{}
	buf := &lineBuffer{}
	out.Init(buf, 0, 8, 2, ' ', 0)

	fmt.Fprintln(out, "variant\tduration\trows/s\tMiB/s\tvs current\tnotes")
	for _, v := range variants {
		d := best[v.name]
		rowsPerSec := float64(rowCount) / d.Seconds()
		mibPerSec := float64(v.bytes) / d.Seconds() / (1024 * 1024)
		speedup := baseline.Seconds() / d.Seconds()

		fmt.Fprintf(out, "%s\t%s\t%s\t%.1f\t%.2fx\t%s\n",
			v.name, d.Round(time.Millisecond), humanCount(rowsPerSec), mibPerSec, speedup, v.notes)
	}
	require.NoError(t, out.Flush())

	t.Logf("\n\n%d rows into %s.%s\n\n%s", rowCount, benchSchema, benchTable, buf.String())
}

// -- harness ----------------------------------------------------------------------

type harness struct {
	pool          *pgxpool.Pool // extended protocol, used for prepared inserts and COPY
	simplePool    *pgxpool.Pool // simple protocol, used for prebuilt literal SQL
	libpq         *sql.DB
	columns       []pgcopy.Column
	binaryCopySQL string
	csvCopySQL    string
	dataDir       string
}

func newHarness(t *testing.T, ctx context.Context) *harness {
	t.Helper()

	image := envString("PGBENCH_PG_IMAGE", "postgres:17-alpine")
	container, err := tcpostgres.Run(ctx, image,
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

	dsn := container.MustConnectionString(ctx, "sslmode=disable")

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	simpleConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	simpleConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	simplePool, err := pgxpool.NewWithConfig(ctx, simpleConfig)
	require.NoError(t, err)
	t.Cleanup(simplePool.Close)

	libpqDB, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = libpqDB.Close() })

	_, err = pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+benchSchema)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, createBenchTableSQL)
	require.NoError(t, err)

	if os.Getenv("PGBENCH_WITH_INDEX") != "" {
		t.Log("creating secondary indexes: COPY's advantage shrinks when index maintenance dominates")
		_, err = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS transfers_id_idx ON bench.transfers (id)`)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS transfers_block_idx ON bench.transfers (_block_number_)`)
		require.NoError(t, err)
	}

	// Resolve the type OIDs from the live catalog. Binary COPY does no coercion, so
	// these must be the server's own, never derived from the declared type names.
	all, err := pgcopy.LoadColumns(ctx, pool, benchSchema, benchTable)
	require.NoError(t, err)
	require.Len(t, all, len(benchColumnNames))
	for i, col := range all {
		require.Equal(t, benchColumnNames[i], col.Name, "column %d out of order", i)
	}

	dataDir := os.Getenv("PGBENCH_KEEP_DATA_DIR")
	if dataDir == "" {
		dataDir = t.TempDir()
	} else {
		require.NoError(t, os.MkdirAll(dataDir, 0o755))
	}

	return &harness{
		pool:          pool,
		simplePool:    simplePool,
		libpq:         libpqDB,
		columns:       all,
		binaryCopySQL: pgcopy.CopySQL(benchSchema, benchTable, all),
		csvCopySQL:    copyCSVSQL(all),
		dataDir:       dataDir,
	}
}

type artifacts struct {
	binaryPath   string
	csvPath      string
	multiRowPath string

	binaryBytes   int64
	csvBytes      int64
	multiRowBytes int64
}

// materialise writes every on-disk artifact in full. Nothing below this point should
// touch the dataset except to hand already-encoded bytes to the server.
func (h *harness) materialise(t *testing.T, ctx context.Context, rows []*row) artifacts {
	t.Helper()

	art := artifacts{
		binaryPath:   filepath.Join(h.dataDir, "data.pgcopy"),
		csvPath:      filepath.Join(h.dataDir, "data.csv"),
		multiRowPath: filepath.Join(h.dataDir, "data.multirow.frames"),
	}

	require.NoError(t, h.writeBinaryCopy(art.binaryPath, rows))
	require.NoError(t, writeCSV(art.csvPath, rows))
	require.NoError(t, writeMultiRowSQL(art.multiRowPath, rows, multiRowBatchSize))

	art.binaryBytes = fileSize(t, art.binaryPath)
	art.csvBytes = fileSize(t, art.csvPath)
	art.multiRowBytes = fileSize(t, art.multiRowPath)

	return art
}

func (h *harness) writeBinaryCopy(path string, rows []*row) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer file.Close()

	writer, err := pgcopy.NewWriter(file, h.columns)
	if err != nil {
		return fmt.Errorf("creating pgcopy writer: %w", err)
	}

	for i, r := range rows {
		values := r.values()
		if err := pgcopy.NormalizeRow(h.columns, values); err != nil {
			return fmt.Errorf("normalizing row %d: %w", i, err)
		}
		if err := writer.WriteRow(values); err != nil {
			return fmt.Errorf("writing row %d: %w", i, err)
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	return file.Sync()
}

// -- variants ---------------------------------------------------------------------

func (h *harness) insertPerRow(ctx context.Context, rows []*row) error {
	statement := insertPlaceholderSQL()

	return h.inTx(ctx, h.pool, func(tx pgx.Tx) error {
		for i, r := range rows {
			values := r.values()
			if err := pgcopy.NormalizeRow(h.columns, values); err != nil {
				return fmt.Errorf("normalizing row %d: %w", i, err)
			}
			if _, err := tx.Exec(ctx, statement, values...); err != nil {
				return fmt.Errorf("inserting row %d: %w", i, err)
			}
		}

		return nil
	})
}

func (h *harness) insertMultiRowBuilt(ctx context.Context, rows []*row) error {
	return h.inTx(ctx, h.simplePool, func(tx pgx.Tx) error {
		for start := 0; start < len(rows); start += multiRowBatchSize {
			end := min(start+multiRowBatchSize, len(rows))
			if _, err := tx.Exec(ctx, buildMultiRowInsert(rows[start:end])); err != nil {
				return fmt.Errorf("inserting rows %d..%d: %w", start, end, err)
			}
		}

		return nil
	})
}

func (h *harness) insertMultiRowBuiltLibpq(ctx context.Context, rows []*row) error {
	tx, err := h.libpq.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for start := 0; start < len(rows); start += multiRowBatchSize {
		end := min(start+multiRowBatchSize, len(rows))
		if _, err := tx.ExecContext(ctx, buildMultiRowInsert(rows[start:end])); err != nil {
			return fmt.Errorf("inserting rows %d..%d: %w", start, end, err)
		}
	}

	return tx.Commit()
}

func (h *harness) insertMultiRowPrebuilt(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close()

	frames := newFrameReader(file)

	return h.inTx(ctx, h.simplePool, func(tx pgx.Tx) error {
		for {
			statement, err := frames.next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("reading statement: %w", err)
			}
			if _, err := tx.Exec(ctx, string(statement)); err != nil {
				return fmt.Errorf("executing prebuilt statement: %w", err)
			}
		}
	})
}

// copyFromFile is the shape the spill design uses at flush time: hand the file to the
// connection and let it shovel bytes. No encoding, no escaping, no parsing client-side.
func (h *harness) copyFromFile(ctx context.Context, path, statement string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close()

	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Conn().PgConn().CopyFrom(ctx, file, statement); err != nil {
		return fmt.Errorf("copy from %s: %w", filepath.Base(path), err)
	}

	return nil
}

func (h *harness) copyFromValues(ctx context.Context, rows []*row) error {
	names := make([]string, len(h.columns))
	for i, col := range h.columns {
		names[i] = col.Name
	}

	var encodeErr error
	source := pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		values := rows[i].values()
		if err := pgcopy.NormalizeRow(h.columns, values); err != nil {
			encodeErr = err
			return nil, err
		}
		return values, nil
	})

	if _, err := h.pool.CopyFrom(ctx, pgx.Identifier{benchSchema, benchTable}, names, source); err != nil {
		return fmt.Errorf("pgx copy from: %w (encode: %v)", err, encodeErr)
	}

	return nil
}

// -- helpers ----------------------------------------------------------------------

func (h *harness) inTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (h *harness) truncate(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := h.pool.Exec(ctx, fmt.Sprintf("TRUNCATE %s.%s", benchSchema, benchTable))
	require.NoError(t, err)
}

func (h *harness) checksum(t *testing.T, ctx context.Context) checksum {
	t.Helper()

	var out checksum
	err := h.pool.QueryRow(ctx, checksumSQL).Scan(
		&out.Count, &out.BlockSum, &out.LogIndexSum, &out.AmountSum,
		&out.TopicsSum, &out.HashLenSum, &out.HashByte0Sum, &out.HashByteLastSum,
		&out.SuccessCount, &out.FeeAbove500, &out.MetaKindCount,
	)
	require.NoError(t, err)

	return out
}

func insertPlaceholderSQL() string {
	placeholders := ""
	quoted := ""
	for i, name := range benchColumnNames {
		if i > 0 {
			placeholders += ", "
			quoted += ", "
		}
		placeholders += "$" + strconv.Itoa(i+1)
		quoted += `"` + name + `"`
	}

	return fmt.Sprintf(`INSERT INTO %s.%s (%s) VALUES (%s)`, benchSchema, benchTable, quoted, placeholders)
}

func copyCSVSQL(cols []pgcopy.Column) string {
	quoted := ""
	for i, col := range cols {
		if i > 0 {
			quoted += ", "
		}
		quoted += pgx.Identifier{col.Name}.Sanitize()
	}

	return fmt.Sprintf("COPY %s (%s) FROM STDIN (FORMAT CSV)",
		pgx.Identifier{benchSchema, benchTable}.Sanitize(), quoted)
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)

	return info.Size()
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()

	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	require.NoError(t, err, "invalid %s", name)

	return value
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func humanBytes(n int64) string {
	f := float64(n)
	for _, unit := range []string{"B", "KiB", "MiB", "GiB"} {
		if f < 1024 {
			return fmt.Sprintf("%.1f%s", f, unit)
		}
		f /= 1024
	}

	return fmt.Sprintf("%.1fTiB", f)
}

func humanCount(f float64) string {
	switch {
	case f >= 1_000_000:
		return fmt.Sprintf("%.2fM", f/1_000_000)
	case f >= 1_000:
		return fmt.Sprintf("%.0fk", f/1_000)
	default:
		return fmt.Sprintf("%.0f", f)
	}
}

// lineBuffer is a tiny io.Writer so the tabwriter output can go into one t.Logf call
// and stay together in the test output.
type lineBuffer struct{ b []byte }

func (l *lineBuffer) Write(p []byte) (int, error) { l.b = append(l.b, p...); return len(p), nil }
func (l *lineBuffer) String() string              { return string(l.b) }
