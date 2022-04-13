package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/abourget/llerrgroup"
	"github.com/jmoiron/sqlx"
	"github.com/streamingfast/substreams/graph-node/subgraph"
	"go.uber.org/zap"
)

func InitiateSchema(ctx context.Context, db *sqlx.DB, subgraph *subgraph.Definition, schema string, logger *zap.Logger) error {
	logger.Info("dropping indexes")
	eg := llerrgroup.New(20)

	err := subgraph.DDL.InitiateSchema(func(statement string) error {
		_, err := db.ExecContext(ctx, strings.ReplaceAll(statement, "%%SCHEMA%%", schema))
		if err != nil {
			return fmt.Errorf("failed to execute index statement %s: %w", statement, err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error: %w", err)
	}

	if err != nil {
		return fmt.Errorf("launch err: %w", err)
	}

	return nil
}

func CreateTables(ctx context.Context, db *sqlx.DB, subgraph *subgraph.Definition, schema string, logger *zap.Logger) error {
	logger.Info("Creating table")
	eg := llerrgroup.New(20)

	err := subgraph.DDL.CreateTables(func(table string, statement string) error {
		return execStatement(ctx, db, strings.ReplaceAll(statement, "%%SCHEMA%%", schema), eg)
	})

	logger.Info("waiting for creation group to complete", zap.Int("group_size", eg.CallsCount()))

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error: %w", err)
	}

	if err != nil {
		return fmt.Errorf("launch err: %w", err)
	}

	return nil
}

func DropIndexes(ctx context.Context, db *sqlx.DB, subgraph *subgraph.Definition, schema string, onlyTables []string, logger *zap.Logger) error {
	logger.Info("dropping indexes")
	eg := llerrgroup.New(20)

	tableFilter := map[string]bool{}
	for _, tbl := range onlyTables {
		tableFilter[tbl] = true
	}

	err := subgraph.DDL.DropIndexes(func(table string, statement string) error {
		if len(onlyTables) > 0 {
			if _, found := tableFilter[table]; !found {
				return nil
			}
		}

		stmt := strings.ReplaceAll(statement, "%%SCHEMA%%", schema)
		logger.Info("dropping index", zap.String("table", table), zap.String("statement", stmt))

		return execStatement(ctx, db, stmt, eg)
	})

	logger.Info("waiting for all indexes to be dropped")
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error: %w", err)
	}

	if err != nil {
		return err
	}

	return nil
}

func CreateIndexes(ctx context.Context, db *sqlx.DB, subgraph *subgraph.Definition, schema string, onlyTables []string, logger *zap.Logger) error {
	logger.Info("creating indexes")
	eg := llerrgroup.New(250)

	tableFilter := map[string]bool{}
	for _, tbl := range onlyTables {
		tableFilter[tbl] = true
	}

	err := subgraph.DDL.CreateIndexes(func(table string, statement string) error {
		if len(onlyTables) > 0 {
			if _, found := tableFilter[table]; !found {
				return nil
			}
		}
		return execStatement(ctx, db, strings.ReplaceAll(statement, "%%SCHEMA%%", schema), eg)
	})

	logger.Info("Create index eg waiting")
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("eg error: %w", err)
	}

	if err != nil {
		return fmt.Errorf("launch err: %w", err)
	}

	logger.Info("Create index eg wait done")

	return nil
}

func execStatement(ctx context.Context, db *sqlx.DB, statement string, eg *llerrgroup.Group) error {
	if eg.Stop() {
		return fmt.Errorf("llerrgrp stop")
	}

	stmt := statement
	eg.Go(func() error {
		// TODO: should we do this in a pool?
		_, err := db.ExecContext(ctx, statement)
		if err != nil {
			return fmt.Errorf("failed to execute index statement %s: %w", stmt, err)
		}
		return nil
	})

	return nil
}
