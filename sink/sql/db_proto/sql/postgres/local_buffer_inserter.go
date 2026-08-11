package postgres

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/buffer"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"go.uber.org/zap"
)

// localBufferInserter routes rows into the local buffer on disk — the one
// `--local-buffer-max-size` bounds — instead of into the database. It is unrelated to
// sql.BufferedInserter, which only records a walk's inserts in memory for replay.
//
// It sits behind the same pgInserter/pgFlusher interfaces as the accumulator, so the
// sinker is unchanged: what differs is that a "flush" only seals a segment, and the
// database is written to later, by the buffer's own goroutine. That is the whole point —
// the stream stops waiting on PostgreSQL.
type localBufferInserter struct {
	buffer *buffer.Buffer
	logger *zap.Logger
}

func newLocalBufferInserter(ctx context.Context, database *Database, options buffer.Options, logger *zap.Logger) (*localBufferInserter, error) {
	applier := buffer.NewApplier(database.pool, database.schema.Name, logger)
	if err := applier.EnsureSchema(ctx); err != nil {
		return nil, err
	}

	tables, err := loadColumnLayouts(ctx, database)
	if err != nil {
		return nil, err
	}

	buf, err := buffer.New(ctx, options, applier, database.schema.Name, tables, logger)
	if err != nil {
		return nil, err
	}

	return &localBufferInserter{buffer: buf, logger: logger.Named("local_buffer_inserter")}, nil
}

// loadColumnLayouts resolves every table's column order and type OIDs from the live
// catalog. Binary COPY does no coercion, so these must be the server's own rather than
// anything derived from the declared type names.
func loadColumnLayouts(ctx context.Context, database *Database) (map[string]*pgcopy.Table, error) {
	layouts := map[string]*pgcopy.Table{}

	names := []string{"_blocks_"}
	for _, table := range database.dialect.GetTables() {
		names = append(names, table.Name)
	}

	for _, name := range names {
		// Resolve through the same reference the dialect writes into its DDL, so the
		// server applies its own identifier folding rather than us guessing at it.
		resolved, err := pgcopy.ResolveTable(ctx, database.pool, tableName(database.schema.Name, name))
		if err != nil {
			return nil, fmt.Errorf("resolving the column layout of %q: %w", name, err)
		}
		layouts[name] = resolved
	}

	return layouts, nil
}

func (i *localBufferInserter) insert(table string, values []any, database *Database) error {
	switch table {
	case "_cursor_":
		// The cursor is not a row to buffer: it is what makes a segment resumable, and
		// it is written by the applier in the same transaction as the segment.
		cursor, ok := values[1].(string)
		if !ok {
			return fmt.Errorf("expected a string cursor, got %T", values[1])
		}
		i.buffer.RecordCursor(cursor)
		return nil

	case "_blocks_":
		blockNum, ok := values[0].(uint64)
		if !ok {
			return fmt.Errorf("expected a uint64 block number, got %T", values[0])
		}
		i.buffer.RecordBlock(blockNum)
	}

	return i.buffer.Insert(table, values)
}

func (i *localBufferInserter) flush(database *Database) error {
	return i.buffer.MaybeSeal(context.Background())
}

func (i *localBufferInserter) close(ctx context.Context) error {
	return i.buffer.Close(ctx)
}
