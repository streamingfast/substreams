package clickhouse

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"go.uber.org/zap"
)

// chApplier sends a sealed segment to ClickHouse and then advances the cursor, which is
// exactly the order the sink has always written in.
//
// ClickHouse has no transactions and keeps its cursor in a file, so a crash between the
// inserts and the cursor write re-streams those blocks and re-inserts those rows. That is
// the guarantee the sink already ships with; the spool does not improve on it and does not
// have to. What it must not do is make it worse, and it does not: applying a segment is
// the same two steps in the same order.
type chApplier struct {
	database *Database
	inserter *AccumulatorInserter
	logger   *zap.Logger
}

func newCHApplier(database *Database, inserter *AccumulatorInserter, logger *zap.Logger) *chApplier {
	return &chApplier{database: database, inserter: inserter, logger: logger.Named("spool_applier")}
}

// EnsureSchema has nothing to create: recovery reads the cursor the sink already stores.
func (a *chApplier) EnsureSchema(context.Context) error { return nil }

// AlreadyApplied answers from how far the stored cursor got.
//
// Without transactions there is no exact answer, and none is needed: replaying a segment
// the database had in fact taken duplicates precisely the rows re-streaming it would have
// duplicated. Skipping what the cursor already covers is what keeps a restart from redoing
// the whole spool.
func (a *chApplier) AlreadyApplied(_ context.Context, manifest *spool.Manifest) (bool, error) {
	cursor, err := a.database.FetchCursor()
	if err != nil || cursor == nil || cursor.IsBlank() {
		//nolint:nilerr // a missing or unreadable cursor means nothing has been applied
		return false, nil
	}

	return manifest.LastBlock != 0 && manifest.LastBlock <= cursor.Block().Num(), nil
}

// Apply replays one segment's rows into the column builders, sends them, then stores the
// cursor.
func (a *chApplier) Apply(_ context.Context, dir string, manifest *spool.Manifest) error {
	// Tables go in the order the dialect assigns, which is the order the accumulator's own
	// flush uses, so a segment reaches the server the same way an unspooled flush would.
	tables := make([]spool.TableRecord, len(manifest.Tables))
	copy(tables, manifest.Tables)
	slices.SortStableFunc(tables, func(left, right spool.TableRecord) int {
		return cmp.Compare(a.ordinal(left.Name), a.ordinal(right.Name))
	})

	for _, table := range tables {
		if err := a.replayTable(dir, table); err != nil {
			return err
		}
	}

	if err := a.inserter.flush(a.database); err != nil {
		return fmt.Errorf("flushing a spooled segment: %w", err)
	}

	if manifest.Cursor == "" {
		return nil
	}

	cursor, err := sink.NewCursor(manifest.Cursor)
	if err != nil {
		return fmt.Errorf("parsing the cursor of a spooled segment: %w", err)
	}

	// Straight to the file: StoreCursor would route it back into the spool, which is where
	// this cursor came from.
	return a.database.storeCursorFile(cursor)
}

func (a *chApplier) ordinal(table string) int {
	if accumulator, found := a.inserter.accumulators[table]; found {
		return accumulator.ordinal
	}

	return len(a.inserter.accumulators)
}

func (a *chApplier) replayTable(dir string, table spool.TableRecord) error {
	path := filepath.Join(dir, table.File)

	reader, err := spool.OpenFrameReader(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer reader.Close()

	var replayed int64
	for {
		encoded, err := reader.ReadField()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		values, err := decodeValues(encoded)
		if err != nil {
			return fmt.Errorf("decoding a row of %q: %w", table.Name, err)
		}

		if err := a.inserter.insert(table.Name, values); err != nil {
			return fmt.Errorf("replaying a row of %q: %w", table.Name, err)
		}
		replayed++
	}

	if replayed != table.Rows {
		return fmt.Errorf("replayed %d rows of %q but the manifest recorded %d", replayed, table.Name, table.Rows)
	}

	return nil
}
