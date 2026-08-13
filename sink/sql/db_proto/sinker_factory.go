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
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	stats2 "github.com/streamingfast/substreams/sink/sql/db_proto/stats"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type SinkerFactoryFunc func(ctx context.Context, dsnString, schemaName string, logger *zap.Logger, tracer logging.Tracer) (*Sinker, error)

type SinkerFactoryOptions struct {
	UseProtoOption bool
	// Constraints says which constraints the schema gets and when; the zero value means
	// all of them, created once the backfill is done.
	Constraints     protosql.ConstraintPolicy
	UseTransactions bool
	// WriteMode says how a sealed spool segment reaches the database. The zero value
	// resolves per driver and schema.
	WriteMode protosql.WriteMode
	// DecodeWorkers bounds how many blocks are unmarshalled and walked concurrently at
	// flush time. Zero picks one per core, less one, capped at 8.
	DecodeWorkers int
	// DecodeBatchSize is how many blocks are held in memory and decoded together. Zero
	// picks four per decode worker. It sizes the CPU stage, not the database write.
	DecodeBatchSize int
	Encoding        bytes.Encoding
	// Spool, when set, holds rows on disk and applies them from a background goroutine.
	Spool      *spool.Options
	Clickhouse SinkerFactoryClickhouse
}

type SinkerFactoryClickhouse struct {
	SinkInfoFolder  string
	CursorFilePath  string
	QueryRetryCount int
	QueryRetrySleep time.Duration
}

func (o SinkerFactoryOptions) Defaults() SinkerFactoryOptions {
	o.DecodeWorkers = ResolveDecodeWorkers(o.DecodeWorkers)
	if o.DecodeBatchSize <= 0 {
		// Four per worker: enough that the slowest block in a batch does not leave the
		// others idle, small enough that the held payloads and their decoded rows stay a
		// bounded amount of memory.
		o.DecodeBatchSize = 4 * o.DecodeWorkers
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
	options = options.Defaults()

	return func(ctx context.Context, dsnString string, schemaName string, logger *zap.Logger, tracer logging.Tracer) (*Sinker, error) {
		database, err := SetupDatabaseSchema(ctx, dsnString, schemaName, outputModuleName, rootMessageDescriptor, options, logger, tracer)
		if err != nil {
			return nil, err
		}

		err = database.Open()
		if err != nil {
			return nil, fmt.Errorf("opening database: %w", err)
		}

		// Before anything is streamed, and concurrently, so a restart onto a loaded table
		// neither waits for a lock nor takes one.
		if err := database.EnsureBlockNumberIndexes(ctx); err != nil {
			return nil, fmt.Errorf("creating the block number indexes: %w", err)
		}

		warnAboutMissingConstraints(database, options.Constraints, logger)

		return NewSinker(
			rootMessageDescriptor,
			baseSink,
			database,
			options.UseTransactions,
			options.Constraints,
			options.DecodeBatchSize,
			options.DecodeWorkers,
			stats2.NewStats(logger, options.DecodeBatchSize),
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
		pgDatabase, pgErr := postgres.NewDatabase(schema, dsn, outputModuleName, rootMessageDescriptor, options.UseProtoOption, options.Constraints, options.Encoding, logger)
		if pgErr != nil {
			return nil, fmt.Errorf("creating postgres database: %w", pgErr)
		}
		if options.Spool != nil {
			pgDatabase.WithSpool(*options.Spool)
		}
		if err := pgDatabase.WithWriteMode(options.WriteMode); err != nil {
			return nil, err
		}
		database = pgDatabase

	case "clickhouse":
		if options.WriteMode != "" && options.WriteMode != protosql.WriteModeAuto && options.WriteMode != protosql.WriteModeBatchInsert {
			return nil, fmt.Errorf("--write-mode=%s is not available on ClickHouse, whose inserts are columnar and typed rather than SQL text; use %q or %q",
				options.WriteMode, protosql.WriteModeAuto, protosql.WriteModeBatchInsert)
		}

		chDatabase, err := clickhouse.NewDatabase(
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
		if options.Spool != nil {
			chDatabase.WithSpool(*options.Spool)
		}
		database = chDatabase

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
		err = database.CreateDatabase(options.Constraints.ApplyUpfront())
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
		if options.Constraints.ApplyUpfront() {
			// The schema exists, but nothing says it carries constraints: a run without
			// them creates the very same tables, and the sink info hash is computed over
			// the DDL the dialect would emit either way. Adding the missing ones is the
			// only way to get there, and doing it here means constraints on a
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

	// The pass owns its own transactions, committing as it goes so that a run killed
	// part-way keeps what it finished.
	if err := database.ApplyConstraints(); err != nil {
		return fmt.Errorf("applying constraints: %w", err)
	}

	logger.Info("constraints applied", zap.Duration("duration", time.Since(startAt)))

	return nil
}

// warnAboutMissingConstraints says on every start whether the schema carries the
// constraints the flags describe.
//
// A run that was interrupted before the backfill ended, or whose constraint pass was
// killed part-way, leaves a database with no primary keys and no foreign keys — which
// answers queries, slowly and without rejecting anything, and looks exactly like a
// database that is fine. The check is one indexed catalog query, so it costs nothing next
// to what it reports on.
//
// It only reports. Creating them is still the constraint timing's business: auto does it
// when the backfill ends, manual leaves it to `sink postgres constraints apply`.
func warnAboutMissingConstraints(database protosql.Database, constraints protosql.ConstraintPolicy, logger *zap.Logger) {
	if constraints.SkipsEverything() {
		return
	}

	missing, err := database.MissingConstraints()
	if err != nil {
		logger.Debug("could not check which constraints the schema carries", zap.Error(err))
		return
	}
	if len(missing) == 0 {
		return
	}

	next := "run `sink postgres constraints apply <manifest>` when a maintenance window suits"
	if constraints.ApplyAtHead() {
		next = "they will be created once the backfill reaches chain HEAD"
	}

	logger.Warn("the schema is missing constraints it is meant to have, so its tables have no indexes to match and reject nothing. "+next,
		zap.Int("missing_count", len(missing)),
		zap.Strings("missing", missing))
}
