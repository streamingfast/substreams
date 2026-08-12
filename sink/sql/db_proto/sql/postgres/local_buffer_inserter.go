package postgres

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"go.uber.org/zap"
)

// spoolInserter routes rows into the on-disk spool instead of into the database. It is
// unrelated to sql.BufferedInserter, which only records a walk's inserts in memory for
// replay.
//
// It sits behind the same pgInserter/pgFlusher interfaces as the accumulator, so the
// sinker is unchanged: what differs is that a "flush" only seals a segment, and the
// database is written to later, by the spool's own goroutine. That is the whole point —
// the stream stops waiting on PostgreSQL.
type localBufferInserter struct {
	buffer *spool.Spool
	logger *zap.Logger
}

func newLocalBufferInserter(ctx context.Context, database *Database, format spool.Format, options spool.Options, logger *zap.Logger) (*localBufferInserter, error) {
	copyRanks, err := database.dialect.TableApplyRanks()
	if err != nil {
		// A cyclic foreign key graph has no table order, which is exactly the schema
		// row-insert mode exists for: its segments are an interleaved log replayed in walk
		// order, so nothing needs ordering by table.
		if format != spool.FormatRowLog {
			return nil, err
		}
		copyRanks = nil
	}

	applier := newPGApplier(database.pool, database.schema.Name, copyRanks, logger)
	if err := applier.EnsureSchema(ctx); err != nil {
		return nil, err
	}

	tables, err := loadColumnLayouts(ctx, database)
	if err != nil {
		return nil, err
	}

	codec := newPGCodec(format, tables, database.dialect.bytesEncoding)

	buf, err := spool.New(ctx, options, codec, applier, database.schema.Name, logger)
	if err != nil {
		return nil, err
	}

	return &localBufferInserter{buffer: buf, logger: logger.Named("spool_inserter")}, nil
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
