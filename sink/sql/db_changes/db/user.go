package db

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

func (l *Loader) CreateUser(ctx context.Context, username string, password string, database string, readOnly bool) (err error) {
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

	err = l.dialect.CreateUser(tx, ctx, l, username, password, database, readOnly)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit db transaction: %w", err)
	}
	l.reset()

	return nil
}
