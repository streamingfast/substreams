package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"go.uber.org/zap"
)

// Applier loads sealed segments into PostgreSQL.
//
// One segment is one transaction: every table file is COPYed, the segment is recorded,
// and the cursor is advanced, all together. Nothing can therefore be half-applied, which
// is what lets recovery decide a segment's fate by looking it up in one table.
type pgApplier struct {
	pool   *pgxpool.Pool
	schema string
	logger *zap.Logger

	// copyRanks orders the table files within a segment: a referenced table has to be
	// COPYed before the tables referencing it, or a foreign key rejects a child row that
	// reaches the server ahead of its parent. It comes from the dialect rather than the
	// manifest so that segments written by an earlier build still apply correctly.
	copyRanks map[string]int

	// applied is the segments table read once, on the first recovery question: first block
	// to last block of every segment the database already holds.
	applied map[uint64]uint64
}

func newPGApplier(pool *pgxpool.Pool, schema string, copyRanks map[string]int, logger *zap.Logger) *pgApplier {
	return &pgApplier{pool: pool, schema: schema, copyRanks: copyRanks, logger: logger.Named("spool_applier")}
}

// copyOrder returns the segment's tables in the order they must be loaded. A table the
// dialect does not know about keeps its manifest position, after the ones it does.
func (a *pgApplier) copyOrder(tables []spool.TableRecord) []spool.TableRecord {
	ordered := make([]spool.TableRecord, len(tables))
	copy(ordered, tables)

	rank := func(table spool.TableRecord) int {
		if at, found := a.copyRanks[table.Name]; found {
			return at
		}

		return len(a.copyRanks)
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		return rank(ordered[i]) < rank(ordered[j])
	})

	return ordered
}

// EnsureSchema creates the bookkeeping table recovery relies on.
func (a *pgApplier) EnsureSchema(ctx context.Context) error {
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

	return a.dropSegmentsPastCursor(ctx)
}

// dropSegmentsPastCursor removes the segment records the stored cursor does not cover.
//
// The cursor is what a restart resumes from, and Run undoes every row above it before a
// block arrives — so a record reaching past it describes rows that are no longer there.
// Left in place it answers AlreadyApplied for the segment carrying exactly those rows,
// which recovery then discards while replaying the segments behind it, moving the cursor
// over a gap it just created.
//
// Segments sealed before the cursor covering them was recorded are how such a record came
// about, which no longer happens; this also clears the ones a database synced by an earlier
// build is still carrying.
func (a *pgApplier) dropSegmentsPastCursor(ctx context.Context) error {
	var stored string
	err := a.pool.QueryRow(ctx, fmt.Sprintf(`SELECT cursor FROM %s WHERE name = 'cursor'`,
		pgx.Identifier{a.schema, "_cursor_"}.Sanitize())).Scan(&stored)
	if err != nil {
		// No cursor yet means nothing has been applied, so there is nothing to contradict.
		return nil
	}

	cursor, err := sink.NewCursor(stored)
	if err != nil || cursor.IsBlank() {
		//nolint:nilerr // an unreadable cursor is not this pass's to report
		return nil
	}

	tag, err := a.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE last_block > $1`, a.segmentsTable()),
		cursor.Block().Num())
	if err != nil {
		return fmt.Errorf("dropping the segment records past the stored cursor: %w", err)
	}

	if tag.RowsAffected() > 0 {
		a.logger.Info("dropped segment records the stored cursor does not cover",
			zap.Int64("dropped", tag.RowsAffected()), zap.Uint64("cursor_block", cursor.Block().Num()))
	}

	return nil
}

func (a *pgApplier) segmentsTable() string {
	return pgx.Identifier{a.schema, "_segments_"}.Sanitize()
}

// Apply loads one segment. The COPYs run sequentially because a transaction is bound to
// one connection; parallelism, if it is ever needed, belongs between segments.
func (a *pgApplier) Apply(ctx context.Context, dir string, manifest *spool.Manifest) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a rollback after commit is a no-op

	switch manifest.Format {
	case spool.FormatTuples:
		if err := a.applyTuples(ctx, tx, dir, manifest); err != nil {
			return err
		}

	case spool.FormatRowLog:
		if err := a.applyRowLog(ctx, tx, dir, manifest); err != nil {
			return err
		}

	default:
		for _, table := range a.copyOrder(manifest.Tables) {
			if err := a.copyTable(ctx, tx, dir, table); err != nil {
				return err
			}
		}
	}

	// A cursor-only segment has no rows to replay, so recovery has nothing to decide about
	// it and re-applying it would only store the same cursor again. Recording it would
	// also collide on first_block, which every such segment leaves at zero.
	if !manifest.CursorOnly() {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s (first_block, last_block, cursor) VALUES ($1, $2, $3)
			 ON CONFLICT (first_block) DO UPDATE SET last_block = $2, cursor = $3, applied_at = now()`,
			a.segmentsTable(),
		), manifest.FirstBlock, manifest.LastBlock, manifest.Cursor); err != nil {
			return fmt.Errorf("recording the applied segment: %w", err)
		}
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
func (a *pgApplier) copyTable(ctx context.Context, tx pgx.Tx, dir string, table spool.TableRecord) error {
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

// AlreadyApplied answers from the segments table, which the applier writes in the same
// transaction as the rows: with transactions available, a segment either landed whole or
// not at all, so this is exact rather than a best guess.
func (a *pgApplier) AlreadyApplied(ctx context.Context, manifest *spool.Manifest) (bool, error) {
	if manifest.CursorOnly() {
		// Nothing to replay and nothing recorded, so replaying only stores the same
		// cursor again.
		return false, nil
	}

	if a.applied == nil {
		applied, err := a.appliedSegments(ctx)
		if err != nil {
			return false, err
		}
		a.applied = applied
	}

	// Both ends have to match. Segments are keyed by their first block, but the block a
	// segment ends on is whatever the sizer settled on at the time, so a range re-streamed
	// after its segment was discarded comes back starting at the same block and ending
	// somewhere else. Answering from the first block alone would call that one applied and
	// drop every row it carries.
	lastBlock, found := a.applied[manifest.FirstBlock]

	return found && lastBlock == manifest.LastBlock, nil
}

// appliedSegments returns the block range of every segment already in the database, so
// recovery can tell what still needs replaying.
func (a *pgApplier) appliedSegments(ctx context.Context) (map[uint64]uint64, error) {
	rows, err := a.pool.Query(ctx, fmt.Sprintf(`SELECT first_block, last_block FROM %s`, a.segmentsTable()))
	if err != nil {
		return nil, fmt.Errorf("listing applied segments: %w", err)
	}
	defer rows.Close()

	applied := map[uint64]uint64{}
	for rows.Next() {
		var firstBlock, lastBlock uint64
		if err := rows.Scan(&firstBlock, &lastBlock); err != nil {
			return nil, fmt.Errorf("scanning an applied segment: %w", err)
		}
		applied[firstBlock] = lastBlock
	}

	return applied, rows.Err()
}

// clearSegments empties the bookkeeping table.
//
// It is only ever called once the spool has been drained and closed, which leaves no
// segment on disk for a record to answer for: the records exist to tell recovery whether a
// directory it found was already applied, and there are no directories left.
func (a *pgApplier) clearSegments(ctx context.Context) error {
	if _, err := a.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s`, a.segmentsTable())); err != nil {
		return fmt.Errorf("clearing the applied-segments table: %w", err)
	}
	a.applied = nil

	return nil
}
