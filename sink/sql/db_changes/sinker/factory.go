package sinker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/streamingfast/logging"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"go.uber.org/zap"
)

type SinkerFactoryFunc func(ctx context.Context, dsnString string, logger *zap.Logger, tracer logging.Tracer) (*SQLSinker, error)

type SinkerFactoryOptions struct {
	CursorTableName         string
	HistoryTableName        string
	ClickhouseCluster       string
	BatchBlockFlushInterval int
	BatchRowFlushInterval   int
	LiveBlockFlushInterval  int
	OnModuleHashMismatch    string
	HandleReorgs            bool
	FlushRetryCount         int
	FlushRetryDelay         time.Duration

	// StandbyLock, PostgreSQL only: take the writer lock of the cursors table before streaming, and
	// wait as a standby while another sink holds it (see SQLSinker.SetWriterLock).
	StandbyLock             bool
	StandbyLockPollInterval time.Duration
	// StallTimeout stops the sink if no block arrives for that long while live (0 disables it).
	StallTimeout time.Duration
}

func SinkerFactory(
	baseSink *sink.Sinker,
	options SinkerFactoryOptions,
) SinkerFactoryFunc {
	return func(ctx context.Context, dsnString string, logger *zap.Logger, tracer logging.Tracer) (*SQLSinker, error) {
		dsn, err := db.ParseDSN(dsnString)
		if err != nil {
			return nil, fmt.Errorf("parsing dsn: %w", err)
		}

		if options.StallTimeout < 0 {
			return nil, fmt.Errorf("the stall timeout cannot be negative, got %s", options.StallTimeout)
		}
		if options.StandbyLock {
			if dsn.Driver() != "postgres" {
				return nil, db.ErrWriterLockUnsupported
			}
			if options.StandbyLockPollInterval <= 0 {
				return nil, fmt.Errorf("the standby lock poll interval must be positive, got %s", options.StandbyLockPollInterval)
			}
		}

		dbLoader, err := db.NewLoader(
			dsn,
			options.CursorTableName,
			options.HistoryTableName,
			options.ClickhouseCluster,
			options.BatchBlockFlushInterval,
			options.BatchRowFlushInterval,
			options.LiveBlockFlushInterval,
			options.OnModuleHashMismatch,
			&options.HandleReorgs,
			logger,
			tracer,
		)
		if err != nil {
			return nil, fmt.Errorf("creating loader: %w", err)
		}

		if err := dbLoader.LoadTables(dsn.Schema(), options.CursorTableName, options.HistoryTableName); err != nil {
			var e *db.SystemTableError
			if errors.As(err, &e) {
				return nil, fmt.Errorf("error validating the system table: %w. Did you run setup?", err)
			}
			return nil, fmt.Errorf("load tables: %w", err)
		}

		sinker, err := New(baseSink, dbLoader, logger, tracer, options.FlushRetryCount, options.FlushRetryDelay)
		if err != nil {
			return nil, fmt.Errorf("unable to setup SQL sinker: %w", err)
		}
		if options.StandbyLock {
			sinker.SetWriterLock(options.StandbyLockPollInterval)
		}
		sinker.SetStallTimeout(options.StallTimeout)

		return sinker, nil
	}
}
