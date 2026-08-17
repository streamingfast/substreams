package benchmarks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestConstraintCost measures what running the from-proto sink with database constraints
// costs on the binary COPY path, which is the question that decides whether they can be
// on by default.
//
// Constraints used to force the row-at-a-time inserter, so "with constraints" meant a
// tenth of the throughput. Ordering the tables by their foreign keys removed that, and
// what is left is index maintenance and referential checks during the COPY itself. This
// separates the two: primary keys and uniques build indexes, foreign keys check a lookup
// per row against an index that has to exist anyway.
//
// A relational shape rather than one flat table, since foreign keys are the point:
// blocks <- parents <- children, with children also carrying a unique column.
func TestConstraintCost(t *testing.T) {
	requireBenchmark(t)

	ctx := context.Background()
	rowCount := envInt(t, "PGBENCH_ROWS", 250_000)

	variants := []struct {
		name string
		// constraints are applied before the load; afterConstraints once it is done,
		// which is the "backfill bare, then turn them on" workflow.
		constraints      []string
		afterConstraints []string
	}{
		{name: "no constraints"},
		{
			name: "primary keys only",
			constraints: []string{
				`ALTER TABLE cost.blocks ADD CONSTRAINT blocks_pk PRIMARY KEY (number)`,
				`ALTER TABLE cost.parents ADD CONSTRAINT parents_pk PRIMARY KEY (id)`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_pk PRIMARY KEY (id)`,
			},
		},
		{
			name: "primary keys and unique",
			constraints: []string{
				`ALTER TABLE cost.blocks ADD CONSTRAINT blocks_pk PRIMARY KEY (number)`,
				`ALTER TABLE cost.parents ADD CONSTRAINT parents_pk PRIMARY KEY (id)`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_pk PRIMARY KEY (id)`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_ref_unique UNIQUE (ref)`,
			},
		},
		{
			name: "primary keys, unique and foreign keys",
			constraints: []string{
				`ALTER TABLE cost.blocks ADD CONSTRAINT blocks_pk PRIMARY KEY (number)`,
				`ALTER TABLE cost.parents ADD CONSTRAINT parents_pk PRIMARY KEY (id)`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_pk PRIMARY KEY (id)`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_ref_unique UNIQUE (ref)`,
				`ALTER TABLE cost.parents ADD CONSTRAINT parents_fk_block FOREIGN KEY (_block_number_) REFERENCES cost.blocks(number) ON DELETE CASCADE`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_fk_block FOREIGN KEY (_block_number_) REFERENCES cost.blocks(number) ON DELETE CASCADE`,
				`ALTER TABLE cost.children ADD CONSTRAINT children_fk_parent FOREIGN KEY (parent_id) REFERENCES cost.parents(id)`,
			},
		},
	}

	full := variants[len(variants)-1].constraints
	variants = append(variants, struct {
		name             string
		constraints      []string
		afterConstraints []string
	}{name: "loaded bare, constraints added after", afterConstraints: full})

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

	dataDir := t.TempDir()

	type measurement struct {
		name           string
		duration       time.Duration
		insertDuration time.Duration
	}
	var measurements []measurement

	for _, variant := range variants {
		// A fresh schema per variant: an index built before the load is not the same
		// thing as one built after it.
		_, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS cost CASCADE; CREATE SCHEMA cost`)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, createCostTablesSQL)
		require.NoError(t, err)

		for _, statement := range variant.constraints {
			_, err := pool.Exec(ctx, statement)
			require.NoError(t, err, statement)
		}

		// Materialise every file before the clock starts, so what is measured is the
		// transport plus the server's own work, never row generation.
		blocksFile := filepath.Join(dataDir, "blocks.pgcopy")
		parentsFile := filepath.Join(dataDir, "parents.pgcopy")
		childrenFile := filepath.Join(dataDir, "children.pgcopy")

		blockCount := rowCount / 100
		if blockCount < 1 {
			blockCount = 1
		}

		writeCopyFile(t, ctx, pool, "blocks", blocksFile, func(write func(...any)) {
			for i := 0; i < blockCount; i++ {
				write(int32(i), fmt.Sprintf("hash-%d", i))
			}
		})
		writeCopyFile(t, ctx, pool, "parents", parentsFile, func(write func(...any)) {
			for i := 0; i < rowCount; i++ {
				write(int32(i%blockCount), fmt.Sprintf("parent-%d", i), fmt.Sprintf("name-%d", i))
			}
		})
		writeCopyFile(t, ctx, pool, "children", childrenFile, func(write func(...any)) {
			for i := 0; i < rowCount; i++ {
				write(int32(i%blockCount), fmt.Sprintf("child-%d", i), fmt.Sprintf("parent-%d", i), fmt.Sprintf("ref-%d", i), int64(i))
			}
		})

		// Topological order, exactly as the applier loads a segment.
		startAt := time.Now()
		copyFile(t, ctx, pool, "blocks", blocksFile)
		copyFile(t, ctx, pool, "parents", parentsFile)
		copyFile(t, ctx, pool, "children", childrenFile)

		for _, statement := range variant.afterConstraints {
			_, err := pool.Exec(ctx, statement)
			require.NoError(t, err, statement)
		}
		elapsed := time.Since(startAt)

		var children int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM cost.children`).Scan(&children))
		require.Equal(t, rowCount, children, "every variant must load the same data")

		// The same rows through the other write path: multi-row INSERT statements, built
		// before the clock starts, in the same table order.
		_, err = pool.Exec(ctx, `DROP SCHEMA IF EXISTS cost CASCADE; CREATE SCHEMA cost`)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, createCostTablesSQL)
		require.NoError(t, err)
		for _, statement := range variant.constraints {
			_, err := pool.Exec(ctx, statement)
			require.NoError(t, err, statement)
		}

		statements := buildInsertStatements(rowCount, blockCount)

		insertStart := time.Now()
		for _, statement := range statements {
			_, err := pool.Exec(ctx, statement)
			require.NoError(t, err)
		}
		for _, statement := range variant.afterConstraints {
			_, err := pool.Exec(ctx, statement)
			require.NoError(t, err, statement)
		}
		insertElapsed := time.Since(insertStart)

		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM cost.children`).Scan(&children))
		require.Equal(t, rowCount, children, "both paths must load the same data")

		measurements = append(measurements, measurement{name: variant.name, duration: elapsed, insertDuration: insertElapsed})
	}

	baseline := measurements[0].duration
	insertBaseline := measurements[0].insertDuration
	t.Log("")
	t.Logf("%d blocks, %d parents, %d children per variant", rowCount/100, rowCount, rowCount)
	t.Logf("%-38s %10s %9s %10s %9s %11s", "variant", "COPY", "vs bare", "INSERT", "vs bare", "COPY gain")
	for _, m := range measurements {
		t.Logf("%-38s %10s %8.2fx %10s %8.2fx %10.2fx",
			m.name,
			m.duration.Round(time.Millisecond), baseline.Seconds()/m.duration.Seconds(),
			m.insertDuration.Round(time.Millisecond), insertBaseline.Seconds()/m.insertDuration.Seconds(),
			m.insertDuration.Seconds()/m.duration.Seconds())
	}
}

// writeCopyFile resolves the table's real column layout and writes the rows in PGCOPY
// binary format, the same way the buffer does.
func writeCopyFile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, path string, rows func(write func(...any))) {
	t.Helper()

	columns, err := pgcopy.LoadColumns(ctx, pool, "cost", table)
	require.NoError(t, err)

	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	writer, err := pgcopy.NewWriter(file, columns)
	require.NoError(t, err)

	var writeErr error
	rows(func(values ...any) {
		if writeErr != nil {
			return
		}
		if err := pgcopy.NormalizeRow(columns, values); err != nil {
			writeErr = err
			return
		}
		writeErr = writer.WriteRow(values)
	})
	require.NoError(t, writeErr)
	require.NoError(t, writer.Close())
}

func copyFile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, path string) {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	_, err = conn.Conn().PgConn().CopyFrom(ctx, file, fmt.Sprintf(`COPY cost.%s FROM STDIN (FORMAT BINARY)`, table))
	require.NoError(t, err)
}

const createCostTablesSQL = `
	CREATE TABLE cost.blocks (number INTEGER NOT NULL, hash TEXT NOT NULL);
	CREATE TABLE cost.parents (_block_number_ INTEGER NOT NULL, id TEXT NOT NULL, name TEXT NOT NULL);
	CREATE TABLE cost.children (_block_number_ INTEGER NOT NULL, id TEXT NOT NULL, parent_id TEXT NOT NULL, ref TEXT NOT NULL, quantity BIGINT NOT NULL);
`

// buildInsertStatements renders the same rows as multi-row INSERT text, the way the
// accumulator does at flush, in the order the tables have to be loaded.
func buildInsertStatements(rowCount, blockCount int) []string {
	const perStatement = 1000

	var statements []string

	appendRows := func(prefix string, total int, row func(i int) string) {
		for start := 0; start < total; start += perStatement {
			end := start + perStatement
			if end > total {
				end = total
			}

			var b strings.Builder
			b.WriteString(prefix)
			for i := start; i < end; i++ {
				if i > start {
					b.WriteString(",")
				}
				b.WriteString(row(i))
			}
			statements = append(statements, b.String())
		}
	}

	appendRows("INSERT INTO cost.blocks (number, hash) VALUES ", blockCount, func(i int) string {
		return fmt.Sprintf("(%d,'hash-%d')", i, i)
	})
	appendRows("INSERT INTO cost.parents (_block_number_, id, name) VALUES ", rowCount, func(i int) string {
		return fmt.Sprintf("(%d,'parent-%d','name-%d')", i%blockCount, i, i)
	})
	appendRows("INSERT INTO cost.children (_block_number_, id, parent_id, ref, quantity) VALUES ", rowCount, func(i int) string {
		return fmt.Sprintf("(%d,'child-%d','parent-%d','ref-%d',%d)", i%blockCount, i, i, i, i)
	})

	return statements
}
