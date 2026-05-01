package db

import (
	"context"
	"database/sql"
)

// TestTx is a stub Tx implementation used in testing.
type TestTx struct{}

func (t *TestTx) Rollback() error { return nil }
func (t *TestTx) Commit() error   { return nil }
func (t *TestTx) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (t *TestTx) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}
