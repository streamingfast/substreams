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
	ctx = clickhouse.Context(context.Background(), clickhouse.WithStdAsync(false))

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

	rowFlushedCount, err = l.dialect.Flush(tx, ctx, l, outputModuleHash, lastFinalBlock)
	if err != nil {
		return 0, fmt.Errorf("dialect flush: %w", err)
	}

	rowFlushedCount += 1
	if err := l.UpdateCursor(ctx, tx, outputModuleHash, cursor); err != nil {
		return 0, fmt.Errorf("update cursor: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit db transaction: %w", err)
	}
	l.reset()

	// We add + 1 to the table count because the `cursors` table is an implicit table
	l.logger.Debug("flushed table(s) rows to database", zap.Int("table_count", l.entries.Len()+1), zap.Int("row_count", rowFlushedCount), zapx.HumanDuration("took", time.Since(startAt)))
	return rowFlushedCount, nil
}

func (l *Loader) Revert(ctx context.Context, outputModuleHash string, cursor *sink.Cursor, lastValidBlock uint64) error {
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

	if err := l.dialect.Revert(tx, ctx, l, lastValidBlock); err != nil {
		return err
	}

	if err := l.UpdateCursor(ctx, tx, outputModuleHash, cursor); err != nil {
		return fmt.Errorf("update cursor after revert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit db transaction: %w", err)
	}

	l.logger.Debug("reverted changes to database", zap.Uint64("last_valid_block", lastValidBlock))
	return nil
}

func (l *Loader) reset() {
	for entriesPair := l.entries.Oldest(); entriesPair != nil; entriesPair = entriesPair.Next() {
		l.entries.Set(entriesPair.Key, NewOrderedMap[string, *Operation]())
	}
	l.batchOrdinal = 0
}
