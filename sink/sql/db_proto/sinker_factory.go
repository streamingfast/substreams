package db_proto

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/logging"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	clickhouse "github.com/streamingfast/substreams/sink/sql/db_proto/sql/click_house"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/buffer"
	schema2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	stats2 "github.com/streamingfast/substreams/sink/sql/db_proto/stats"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type SinkerFactoryFunc func(ctx context.Context, dsnString, schemaName string, logger *zap.Logger, tracer logging.Tracer) (*Sinker, error)

type SinkerFactoryOptions struct {
	UseProtoOption  bool
	UseConstraints  bool
	UseTransactions bool
	BlockBatchSize  int
	// DecodeWorkers bounds how many blocks are unmarshalled and walked concurrently at
	// flush time. Zero picks one per core, less one.
	DecodeWorkers int
	Encoding      bytes.Encoding
	// LocalBuffer, when set, buffers rows on disk and loads them with binary COPY from a
	// background goroutine. PostgreSQL only.
	LocalBuffer *buffer.Options
	Clickhouse  SinkerFactoryClickhouse
}

type SinkerFactoryClickhouse struct {
	SinkInfoFolder  string
	CursorFilePath  string
	QueryRetryCount int
	QueryRetrySleep time.Duration
}

func (o SinkerFactoryOptions) Defaults() SinkerFactoryOptions {
	if o.BlockBatchSize <= 0 {
		o.BlockBatchSize = 25
	}
	o.UseTransactions = true
	if o.Encoding == 0 {
		o.Encoding = bytes.EncodingRaw
	}
	return o
}

func SinkerFactory(
	baseSink *sink.Sinker,
	outputModuleName string,
	rootMessageDescriptor protoreflect.MessageDescriptor,
	options SinkerFactoryOptions,
) SinkerFactoryFunc {
	return func(ctx context.Context, dsnString string, schemaName string, logger *zap.Logger, tracer logging.Tracer) (*Sinker, error) {
		database, err := SetupDatabaseSchema(ctx, dsnString, schemaName, outputModuleName, rootMessageDescriptor, options, logger, tracer)
		if err != nil {
			return nil, err
		}

		err = database.Open()
		if err != nil {
			return nil, fmt.Errorf("opening database: %w", err)
		}

		return NewSinker(
			rootMessageDescriptor,
			baseSink,
			database,
			options.UseTransactions,
			options.UseConstraints,
			options.BlockBatchSize,
			options.DecodeWorkers,
			stats2.NewStats(logger, options.BlockBatchSize),
			logger,
		), nil
	}
}

// SetupDatabaseSchema constructs the target Database for the given DSN and output
// message descriptor and ensures its schema exists: on a fresh database it creates
// the schema, tables and system tables and records the sink info hash; on an existing
// one it reconciles the sink info hash when a compatible migration is available. It
// returns the ready (but not opened) Database so callers can either start a sinker
// (run path) or stop right after schema setup (setup command). It is idempotent: on a
// database that is already set up with a matching hash, it performs no schema changes.
func SetupDatabaseSchema(
	ctx context.Context,
	dsnString string,
	schemaName string,
	outputModuleName string,
	rootMessageDescriptor protoreflect.MessageDescriptor,
	options SinkerFactoryOptions,
	logger *zap.Logger,
	tracer logging.Tracer,
) (protosql.Database, error) {
	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}

	schema, err := schema2.NewSchema(schemaName, rootMessageDescriptor, options.UseProtoOption, logger)
	if err != nil {
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	var database protosql.Database

	switch dsn.Driver() {
	case "postgres":
		pgDatabase, pgErr := postgres.NewDatabase(schema, dsn, outputModuleName, rootMessageDescriptor, options.UseProtoOption, options.UseConstraints, options.Encoding, logger)
		if pgErr != nil {
			return nil, fmt.Errorf("creating postgres database: %w", pgErr)
		}
		if options.LocalBuffer != nil {
			pgDatabase.WithLocalBuffer(*options.LocalBuffer)
		}
		database = pgDatabase

	case "clickhouse":
		database, err = clickhouse.NewDatabase(
			ctx,
			schema,
			dsn,
			outputModuleName,
			rootMessageDescriptor,
			options.Clickhouse.SinkInfoFolder,
			options.Clickhouse.CursorFilePath,
			true,
			options.Encoding,
			logger,
			tracer,
			options.Clickhouse.QueryRetryCount,
			options.Clickhouse.QueryRetrySleep,
		)
		if err != nil {
			return nil, fmt.Errorf("creating clickhouse database: %w", err)
		}

	default:
		panic(fmt.Sprintf("unsupported driver: %s", dsn.Driver()))

	}

	sinkInfo, err := database.FetchSinkInfo(schema.Name)
	if err != nil {
		return nil, fmt.Errorf("fetching sink info: %w", err)
	}

	logger.Info("sink info read", zap.Reflect("sink_info", sinkInfo))
	if sinkInfo == nil {
		err := database.BeginTransaction()
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		err = database.CreateDatabase(options.UseConstraints)
		if err != nil {
			database.RollbackTransaction()
			return nil, fmt.Errorf("creating database: %w", err)
		}

		err = database.StoreSinkInfo(schemaName, database.GetDialect().SchemaHash())
		if err != nil {
			database.RollbackTransaction()
			return nil, fmt.Errorf("storing sink info: %w", err)
		}

		err = database.CommitTransaction()
		if err != nil {
			return nil, fmt.Errorf("commit transaction: %w", err)
		}

	} else {
		if options.UseConstraints {
			// The schema exists, but nothing says it carries constraints: a run without
			// them creates the very same tables, and the sink info hash is computed over
			// the DDL the dialect would emit either way. Adding the missing ones is the
			// only way to get there, and doing it here means --with-constraints on a
			// database synced without them behaves the same as one created with them.
			if err := applyConstraints(database, logger); err != nil {
				return nil, err
			}
		}

		migrationNeeded := sinkInfo.SchemaHash != database.GetDialect().SchemaHash()
		if migrationNeeded {

			tempSchemaName := schema.Name + "_" + database.GetDialect().SchemaHash()
			tempSinkInfo, err := database.FetchSinkInfo(tempSchemaName)
			if err != nil {
				return nil, fmt.Errorf("fetching temp schema sink info: %w", err)
			}
			if tempSinkInfo != nil {
				hash, err := database.DatabaseHash(schema.Name)
				if err != nil {
					return nil, fmt.Errorf("fetching schema %q hash: %w", schema.Name, err)
				}
				dbTempHash, err := database.DatabaseHash(tempSchemaName)
				if err != nil {
					return nil, fmt.Errorf("fetching temp schema %q hash: %w", tempSchemaName, err)
				}

				if hash != dbTempHash {
					return nil, fmt.Errorf("schema %s and temp schema %s have different hash", schema.Name, tempSchemaName)
				}
				err = database.BeginTransaction()
				if err != nil {
					return nil, fmt.Errorf("begin transaction: %w", err)
				}
				err = database.UpdateSinkInfoHash(schemaName, tempSinkInfo.SchemaHash)
				if err != nil {
					database.RollbackTransaction()
					return nil, fmt.Errorf("updating sink info hash: %w", err)
				}

				err = database.CommitTransaction()
				if err != nil {
					return nil, fmt.Errorf("commit transaction: %w", err)
				}

			} else {
				//todo: create the temp schema ... and exit

				//err = schema.ChangeName(tempSchemaName, dialect)
				//if err != nil {
				//	return nil, fmt.Errorf("changing schema name: %w", err)
				//}
				//generateTempSchema = true
			}
		}
	}

	return database, nil
}

// applyConstraints adds the constraints an earlier run left out, in one transaction.
//
// On a populated database this can run for a long time: every index has to be built and
// every foreign key validated, with the table locked while it happens. Hence the warning
// rather than a silent wait.
func applyConstraints(database protosql.Database, logger *zap.Logger) error {
	logger.Warn("adding the SQL constraints to the existing schema, this can take a long time and locks the tables while it runs")

	startAt := time.Now()
	if err := database.BeginTransaction(); err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := database.ApplyConstraints(); err != nil {
		database.RollbackTransaction()
		return fmt.Errorf("applying constraints: %w", err)
	}

	if err := database.CommitTransaction(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	logger.Info("constraints applied", zap.Duration("duration", time.Since(startAt)))

	return nil
}
