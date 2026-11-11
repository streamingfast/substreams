package db

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/streamingfast/logging/zapx"
	sink "github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
)

func (l *Loader) Flush(ctx context.Context, outputModuleHash string, cursor *sink.Cursor, lastFinalBlock uint64) (rowFlushedCount int, err error) {
	// On PostgreSQL the flush is one transaction, so cancelling it (the sink is stopping, or stalled
	// on a hung call) rolls it all back. ClickHouse writes are not transactional: a flush cut short
	// could leave some tables written without the cursor, so it always runs to the end there.
	if _, transactional := l.dialect.(*PostgresDialect); !transactional {
		// Inserts return once written, whatever the DSN or the server settings say: the cursor
		// must not be taken as stored before it is.
		ctx = clickhouse.Context(context.Background(), clickhouse.WithSettings(clickhouse.Settings{"async_insert": 0}))
	}

	startAt := time.Now()
	tx, err := l.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to being db transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if err := tx.Rollback(); err != nil {
				l.logger.Warn("failed to rollback transaction", zap.Error(err))
			}
		}
	}()

	// Checked first: fails before any write if the stored cursor moved, and its row lock makes a
	// concurrent writer wait, then fail the same check.
	if l.checkingCursor() {
		if err := l.checkCursor(ctx, tx, outputModuleHash); err != nil {
			return 0, err
		}
	}

	rowFlushedCount, err = l.dialect.Flush(tx, ctx, l, outputModuleHash, lastFinalBlock)
	if err != nil {
		return 0, fmt.Errorf("dialect flush: %w", err)
	}

	rowFlushedCount += 1
	cursorStartAt := time.Now()
	if err := l.UpdateCursor(ctx, tx, outputModuleHash, cursor); err != nil {
		return 0, fmt.Errorf("update cursor: %w", err)
	}
	// Timed on its own: on ClickHouse the cursor insert waits for the server to write it, so it
	// can account for a large share of a flush on a slow storage.
	cursorDuration := time.Since(cursorStartAt)
	QueryExecutionDuration.AddInt64(cursorDuration.Nanoseconds(), "cursor")

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit db transaction: %w", err)
	}
	l.recordCursor(cursor)
	if l.detached != nil {
		l.detached = nil
	} else {
		l.reset()
	}

	l.logger.Debug("flushed table(s) rows to database", zap.Int("row_count", rowFlushedCount), zapx.HumanDuration("took", time.Since(startAt)), zapx.HumanDuration("cursor_took", cursorDuration))
	return rowFlushedCount, nil
}

func (l *Loader) Revert(ctx context.Context, outputModuleHash string, cursor *sink.Cursor, lastValidBlock uint64) (err error) {
	tx, err := l.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to being db transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if err := tx.Rollback(); err != nil {
				l.logger.Warn("failed to rollback transaction", zap.Error(err))
			}
		}
	}()

	// See Flush: checked first.
	if l.checkingCursor() {
		if err := l.checkCursor(ctx, tx, outputModuleHash); err != nil {
			return err
		}
	}

	if err := l.dialect.Revert(tx, ctx, l, lastValidBlock); err != nil {
		return err
	}

	// Pending changes are not in the history table yet: drop those of the reverted blocks and write
	// the others along with the cursor, so they are not lost if the sink stops before its next flush.
	dropped := l.dropPendingAfter(lastValidBlock)
	if l.HasPendingEntries() {
		if _, err := l.dialect.Flush(tx, ctx, l, outputModuleHash, cursor.LIB.Num()); err != nil {
			return fmt.Errorf("flush pending changes on revert: %w", err)
		}
	}

	if err := l.UpdateCursor(ctx, tx, outputModuleHash, cursor); err != nil {
		return fmt.Errorf("update cursor after revert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit db transaction: %w", err)
	}
	l.recordCursor(cursor)
	l.reset()

	l.logger.Debug("reverted changes to database", zap.Uint64("last_valid_block", lastValidBlock), zap.Int("dropped_pending_operations", dropped))
	return nil
}

// dropPendingAfter discards pending operations from blocks after lastValidBlock.
func (l *Loader) dropPendingAfter(lastValidBlock uint64) (dropped int) {
	for tablePair := l.entries.Oldest(); tablePair != nil; tablePair = tablePair.Next() {
		var orphaned []string
		for opPair := tablePair.Value.Oldest(); opPair != nil; opPair = opPair.Next() {
			if block := opPair.Value.reversibleBlockNum; block != nil && *block > lastValidBlock {
				orphaned = append(orphaned, opPair.Key)
			}
		}
		for _, key := range orphaned {
			tablePair.Value.Delete(key)
		}
		dropped += len(orphaned)
	}
	return dropped
}

// SupportsBackgroundFlush reports whether Flush can run while changes are added.
func (l *Loader) SupportsBackgroundFlush() bool {
	_, postgres := l.dialect.(*PostgresDialect)
	return postgres
}

// DetachPending sets the pending changes apart for the next Flush, so that new ones can be added
// while it runs. The caller must not call it again before that Flush succeeded.
func (l *Loader) DetachPending() {
	l.detached = l.entries
	entries := NewOrderedMap[string, *OrderedMap[string, *Operation]]()
	for pair := l.detached.Oldest(); pair != nil; pair = pair.Next() {
		entries.Set(pair.Key, NewOrderedMap[string, *Operation]())
	}
	l.entries = entries
	l.batchOrdinal = 0
}

func (l *Loader) pendingForFlush() *OrderedMap[string, *OrderedMap[string, *Operation]] {
	if l.detached != nil {
		return l.detached
	}
	return l.entries
}

func (l *Loader) reset() {
	for entriesPair := l.entries.Oldest(); entriesPair != nil; entriesPair = entriesPair.Next() {
		l.entries.Set(entriesPair.Key, NewOrderedMap[string, *Operation]())
	}
	l.batchOrdinal = 0
}
