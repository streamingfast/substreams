package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"go.uber.org/zap"
)

// Applier loads sealed segments into PostgreSQL.
//
// One segment is one transaction: every table file is COPYed, the segment is recorded,
// and the cursor is advanced, all together. Nothing can therefore be half-applied, which
// is what lets recovery decide a segment's fate by looking it up in one table.
type Applier struct {
	pool   *pgxpool.Pool
	schema string
	logger *zap.Logger
}

func NewApplier(pool *pgxpool.Pool, schema string, logger *zap.Logger) *Applier {
	return &Applier{pool: pool, schema: schema, logger: logger.Named("cache_applier")}
}

// EnsureSchema creates the bookkeeping table recovery relies on.
func (a *Applier) EnsureSchema(ctx context.Context) error {
	statement := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			first_block BIGINT NOT NULL,
			last_block  BIGINT NOT NULL,
			cursor      TEXT   NOT NULL,
			applied_at  TIMESTAMP NOT NULL DEFAULT now(),
			PRIMARY KEY (first_block)
		)`, a.segmentsTable())

	if _, err := a.pool.Exec(ctx, statement); err != nil {
		return fmt.Errorf("creating the applied-segments table: %w", err)
	}

	return nil
}

func (a *Applier) segmentsTable() string {
	return pgx.Identifier{a.schema, "_segments_"}.Sanitize()
}

// Apply loads one segment. The COPYs run sequentially because a transaction is bound to
// one connection; parallelism, if it is ever needed, belongs between segments.
func (a *Applier) Apply(ctx context.Context, dir string, manifest *Manifest) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a rollback after commit is a no-op

	for _, table := range manifest.Tables {
		if err := a.copyTable(ctx, tx, dir, table); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (first_block, last_block, cursor) VALUES ($1, $2, $3)
		 ON CONFLICT (first_block) DO UPDATE SET last_block = $2, cursor = $3, applied_at = now()`,
		a.segmentsTable(),
	), manifest.FirstBlock, manifest.LastBlock, manifest.Cursor); err != nil {
		return fmt.Errorf("recording the applied segment: %w", err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (name, cursor) VALUES ('cursor', $1)
		 ON CONFLICT (name) DO UPDATE SET cursor = $1`,
		pgx.Identifier{a.schema, "_cursor_"}.Sanitize(),
	), manifest.Cursor); err != nil {
		return fmt.Errorf("storing the cursor: %w", err)
	}

	return tx.Commit(ctx)
}

// copyTable streams one pre-encoded file straight into the server. Because the bytes on
// disk are already in the binary COPY wire format, this is a copy from file to socket:
// no encoding, no escaping, no parsing.
func (a *Applier) copyTable(ctx context.Context, tx pgx.Tx, dir string, table TableRecord) error {
	path := filepath.Join(dir, table.File)

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close()

	columns := make([]pgcopy.Column, len(table.Columns))
	for i, name := range table.Columns {
		columns[i] = pgcopy.Column{Name: name}
	}

	statement := pgcopy.CopySQL(table.Schema, table.Relation, columns)

	tag, err := tx.Conn().PgConn().CopyFrom(ctx, file, statement)
	if err != nil {
		return fmt.Errorf("copying %s into %q: %w", table.File, table.Name, err)
	}
	if tag.RowsAffected() != table.Rows {
		return fmt.Errorf("copied %d rows into %q but the manifest recorded %d",
			tag.RowsAffected(), table.Name, table.Rows)
	}

	return nil
}

// AppliedSegments returns the first_block of every segment already in the database, so
// recovery can tell what still needs replaying.
func (a *Applier) AppliedSegments(ctx context.Context) (map[uint64]bool, error) {
	rows, err := a.pool.Query(ctx, fmt.Sprintf(`SELECT first_block FROM %s`, a.segmentsTable()))
	if err != nil {
		return nil, fmt.Errorf("listing applied segments: %w", err)
	}
	defer rows.Close()

	applied := map[uint64]bool{}
	for rows.Next() {
		var firstBlock uint64
		if err := rows.Scan(&firstBlock); err != nil {
			return nil, fmt.Errorf("scanning an applied segment: %w", err)
		}
		applied[firstBlock] = true
	}

	return applied, rows.Err()
}
