package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lithammer/dedent"
	sink "github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
)

var ErrCursorNotFound = errors.New("cursor not found")

type cursorRow struct {
	ID       string
	Cursor   string
	BlockNum uint64
	BlockID  string
}

// GetAllCursors returns an unordered map given for each module's hash recorded
// the active cursor for it.
func (l *Loader) GetAllCursors(ctx context.Context) (out map[string]*sink.Cursor, err error) {
	query := l.dialect.GetAllCursorsQuery(l.cursorTable.identifier)
	rows, err := l.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all cursors: %w", err)
	}

	out = make(map[string]*sink.Cursor)
	for rows.Next() {
		c := &cursorRow{}
		if err := rows.Scan(&c.ID, &c.Cursor, &c.BlockNum, &c.BlockID); err != nil {
			return nil, fmt.Errorf("getting all cursors:  %w", err)
		}

		out[c.ID], err = sink.NewCursor(c.Cursor)
		if err != nil {
			return nil, fmt.Errorf("database corrupted: stored cursor %q is not a valid cursor", c.Cursor)
		}
	}

	return out, nil
}

func (l *Loader) GetCursor(ctx context.Context, outputModuleHash string) (cursor *sink.Cursor, mismatchDetected bool, err error) {
	cursors, err := l.GetAllCursors(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("get cursor: %w", err)
	}

	if len(cursors) == 0 {
		return sink.NewBlankCursor(), false, ErrCursorNotFound
	}

	activeCursor, found := cursors[outputModuleHash]
	if found {
		return activeCursor, false, err
	}

	// It's not found at this point, look for one with highest block, we will report
	// (maybe) a warning if the module hash is different, which is the case here.
	actualOutputModuleHash, activeCursor := cursorAtHighestBlock(cursors)

	switch l.moduleMismatchMode {
	case OnModuleHashMismatchIgnore:
		return activeCursor, true, err

	case OnModuleHashMismatchWarn:
		l.logger.Warn(
			fmt.Sprintf("cursor module hash mismatch, continuing using cursor at highest block %s, this warning can be made silent by using '--on-module-hash-mismatch=ignore'", activeCursor.Block()),
			zap.String("expected_module_hash", outputModuleHash),
			zap.String("actual_module_hash", actualOutputModuleHash),
		)

		return activeCursor, true, err

	case OnModuleHashMismatchError:
		return nil, true, fmt.Errorf("cursor module hash mismatch, refusing to continue because flag '--on-module-hash-mismatch=error' (defaults) is set, you can change to 'warn' or 'ignore': your module's hash is %q but cursor with highest block (%d) module hash is actually %q in the database",
			outputModuleHash,
			activeCursor.Block().Num(),
			actualOutputModuleHash,
		)

	default:
		panic(fmt.Errorf("unknown module mismatch mode %q", l.moduleMismatchMode))
	}
}

func cursorAtHighestBlock(in map[string]*sink.Cursor) (hash string, highest *sink.Cursor) {
	for moduleHash, cursor := range in {
		if highest == nil || cursor.Block().Num() > highest.Block().Num() {
			highest = cursor
			hash = moduleHash
		}
	}

	return
}

func (l *Loader) InsertCursor(ctx context.Context, moduleHash string, c *sink.Cursor) error {
	query := fmt.Sprintf("INSERT INTO %s (id, cursor, block_num, block_id) values ('%s', '%s', %d, '%s')",
		l.cursorTable.identifier,
		moduleHash,
		c,
		c.Block().Num(),
		c.Block().ID(),
	)
	if _, err := l.DB.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("insert cursor: %w", err)
	}

	return nil
}

// UpdateCursor updates the active cursor. If no cursor is active and no update occurred, returns
// ErrCursorNotFound. If the update was not successful on the database, returns an error.
// You can use tx=nil to run the query outside of a transaction.
func (l *Loader) UpdateCursor(ctx context.Context, tx Tx, moduleHash string, c *sink.Cursor) error {
	l.logger.Debug("updating cursor", zap.String("module_hash", moduleHash), zap.Stringer("cursor", c))
	_, err := l.runModifiyQuery(ctx, tx, "update", l.dialect.GetUpdateCursorQuery(
		l.cursorTable.identifier, moduleHash, c, c.Block().Num(), c.Block().ID(),
	))
	return err
}

// DeleteCursor deletes the active cursor for the given 'moduleHash'. If no cursor is active and
// no delete occurrred, returns ErrCursorNotFound. If the delete was not successful on the database, returns an error.
func (l *Loader) DeleteCursor(ctx context.Context, moduleHash string) error {
	_, err := l.runModifiyQuery(ctx, nil, "delete", fmt.Sprintf("DELETE FROM %s WHERE id = '%s'", l.cursorTable.identifier, moduleHash))
	return err
}

// DeleteAllCursors deletes the active cursor for the given 'moduleHash'. If no cursor is active and
// no delete occurrred, returns ErrCursorNotFound. If the delete was not successful on the database, returns an error.
func (l *Loader) DeleteAllCursors(ctx context.Context) (deletedCount int64, err error) {
	deletedCount, err = l.runModifiyQuery(ctx, nil, "delete", fmt.Sprintf("DELETE FROM %s", l.cursorTable.identifier))
	if err != nil && errors.Is(err, ErrCursorNotFound) {
		return 0, nil
	}

	return deletedCount, nil
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// runModifiyQuery runs the logic to execute a query that is supposed to modify the database in some form affecting
// at least 1 row.
//
// If `tx` is nil, we use `l.DB` as the execution context, so an operations happening outside
// a transaction. Otherwise, tx is the execution context.
func (l *Loader) runModifiyQuery(ctx context.Context, tx Tx, action string, query string) (rowsAffected int64, err error) {
	var executor sqlExecutor = l.DB
	if tx != nil {
		executor = tx
	}

	result, err := executor.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s cursor: %w", action, err)
	}

	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}

	if l.dialect.DriverSupportRowsAffected() && rowsAffected <= 0 {
		return 0, ErrCursorNotFound
	}

	return rowsAffected, nil
}

func query(in string, args ...any) string {
	return fmt.Sprintf(strings.TrimSpace(dedent.Dedent(in)), args...)
}
