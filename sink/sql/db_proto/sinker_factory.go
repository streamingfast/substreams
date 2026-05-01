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
	Parallel        bool
	Encoding        bytes.Encoding
	Clickhouse      SinkerFactoryClickhouse
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
			database, err = postgres.NewDatabase(schema, dsn, outputModuleName, rootMessageDescriptor, options.UseProtoOption, options.UseConstraints, options.Encoding, logger)
			if err != nil {
				return nil, fmt.Errorf("creating postgres database: %w", err)
			}

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

		} else {
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
				}
			}
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
			options.Parallel,
			stats2.NewStats(logger),
			logger,
		), nil
	}
}
