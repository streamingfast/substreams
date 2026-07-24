package sinker

import (
	"context"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/streamingfast/logging"
	sinksql "github.com/streamingfast/substreams/sink/sql"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

const (
	deprecated_supportedDeployableService = "type.googleapis.com/sf.substreams.sink.sql.v1.Service"
	supportedDeployableService            = "type.googleapis.com/sf.substreams.sink.sql.service.v1.Service"
)

// SinkerSetupOptions contains configuration for the setup operation
type SinkerSetupOptions struct {
	CursorTableName            string
	HistoryTableName           string
	ClickhouseCluster          string
	OnModuleHashMismatch       string
	SystemTablesOnly           bool
	IgnoreDuplicateTableErrors bool
	Postgraphile               bool
}

// SinkerSetup sets up the required infrastructure for a Substreams SQL sink
func SinkerSetup(
	ctx context.Context,
	dsnString string,
	pkg *pbsubstreams.Package,
	options SinkerSetupOptions,
	logger *zap.Logger,
	tracer logging.Tracer,
) error {
	sinkConfig, err := sinksql.ExtractSinkService(pkg)
	if err != nil {
		return fmt.Errorf("extract sink config: %w", err)
	}

	dsn, err := db2.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	handleReorgs := false
	dbLoader, err := db2.NewLoader(
		dsn,
		options.CursorTableName,
		options.HistoryTableName,
		options.ClickhouseCluster,
		0, 0, 0,
		options.OnModuleHashMismatch,
		&handleReorgs,
		logger, tracer,
	)
	if err != nil {
		return fmt.Errorf("creating loader: %w", err)
	}
	defer dbLoader.Close()

	userSQLSchema := sinkConfig.Schema
	if options.SystemTablesOnly {
		userSQLSchema = ""
	}

	err = dbLoader.Setup(ctx, dsn.Schema(), userSQLSchema, options.Postgraphile)
	if err != nil {
		if isDuplicateTableError(err) && options.IgnoreDuplicateTableErrors {
			logger.Info("received duplicate table error, script did not execute successfully")
		} else {
			return fmt.Errorf("setup: %w", err)
		}
	}
	logger.Info("setup completed successfully")
	return nil
}

// isDuplicateTableError checks if the error is a PostgreSQL duplicate table error
func isDuplicateTableError(err error) bool {
	var sqlError *pq.Error
	if !errors.As(err, &sqlError) {
		return false
	}

	// List at https://www.postgresql.org/docs/14/errcodes-appendix.html#ERRCODES-TABLE
	switch sqlError.Code {
	// Error code named `duplicate_table`
	case "42P07":
		return true
	}

	return false
}
