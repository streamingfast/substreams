package clickhouse

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"time"

	"github.com/ClickHouse/ch-go"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type Database struct {
	*sql.BaseDatabase
	schema          *schema.Schema
	sinkInfoFolder  string
	cursorFilePath  string
	logger          *zap.Logger
	dialect         *DialectClickHouse
	cachedClient    *ch.Client
	dsn             *db.DSN
	ctx             context.Context
	inserter        *AccumulatorInserter
	bytesEncoding   bytes.Encoding
	queryRetryCount int
	queryRetrySleep time.Duration
}

func NewDatabase(
	ctx context.Context,
	schema *schema.Schema,
	dsn *db.DSN,
	moduleOutputType string,
	rootMessageDescriptor protoreflect.MessageDescriptor,
	sinkInfoFolder string,
	cursorFilePath string,
	useProtoOptions bool,
	bytesEncoding bytes.Encoding,
	logger *zap.Logger,
	tracer logging.Tracer,
	queryRetryCount int,
	queryRetrySleep time.Duration,
) (*Database, error) {
	baseDB, err := sql.NewBaseDatabase(moduleOutputType, rootMessageDescriptor, useProtoOptions, logger)
	if err != nil {
		return nil, fmt.Errorf("creating base database: %w", err)
	}
	dialect, err := NewDialectClickHouse(schema, bytesEncoding, logger)
	if err != nil {
		return nil, fmt.Errorf("creating dialect: %w", err)
	}

	database := &Database{
		ctx:             ctx,
		dsn:             dsn,
		BaseDatabase:    baseDB,
		dialect:         dialect,
		schema:          schema,
		sinkInfoFolder:  sinkInfoFolder,
		cursorFilePath:  cursorFilePath,
		logger:          logger,
		bytesEncoding:   bytesEncoding,
		queryRetryCount: queryRetryCount,
		queryRetrySleep: queryRetrySleep,
	}
	if database.queryRetryCount <= 0 {
		database.queryRetryCount = 3
	}
	if database.queryRetrySleep <= 0 {
		database.queryRetrySleep = time.Second
	}
	inserter, err := NewAccumulatorInserter(database, logger, tracer)
	if err != nil {
		return nil, fmt.Errorf("creating accumulator inserter: %w", err)
	}
	database.inserter = inserter

	return database, nil
}

func (d *Database) Open() error {
	return nil
}

func newClient(dsn *db.DSN, logger *zap.Logger) (*ch.Client, error) {
	chOption := ch.Options{
		Address:     fmt.Sprintf("%s:%d", dsn.Host, dsn.Port),
		Database:    dsn.Database,
		User:        dsn.Username,
		Password:    dsn.Password,
		DialTimeout: 30 * time.Second,
	}

	for key, value := range dsn.Options.Iter() {
		if key == "secure" && value == "true" {
			chOption.TLS = &tls.Config{}
			continue
		}
		if key == "username" {
			chOption.User = value
			continue
		}
		if key == "password" {
			chOption.Password = value
			continue
		}
		if key == "compress" && value == "true" {
			chOption.Compression = ch.CompressionLZ4
			continue
		}
	}

	for {
		client, err := ch.Dial(context.Background(), chOption)
		if err != nil {
			logger.Warn("dialing clickhouse failed, will retry", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		return client, nil
	}
}

func (d *Database) client() (*ch.Client, error) {
	if d.cachedClient == nil || d.cachedClient.IsClosed() {
		client, err := newClient(d.dsn, d.logger)
		if err != nil {
			return nil, fmt.Errorf("creating clickhouse client: %w", err)
		}
		d.cachedClient = client

	}

	return d.cachedClient, nil
}

func (d *Database) freshClient() (*ch.Client, error) {
	client, err := newClient(d.dsn, d.logger)
	if err != nil {
		return nil, fmt.Errorf("creating clickhouse client: %w", err)
	}
	d.cachedClient = client
	return client, nil
}

func (d *Database) clientNoCache(dsn *db.DSN) (*ch.Client, error) {
	client, err := newClient(dsn, d.logger)
	if err != nil {
		return nil, fmt.Errorf("creating clickhouse client: %w", err)
	}
	return client, nil
}

func (d *Database) CreateDatabase(useConstraints bool) error {
	dsn := d.dsn.Clone()
	dsn.Database = "default"
	client, err := d.clientNoCache(dsn)
	if err != nil {
		return fmt.Errorf("creating clickhouse client: %w", err)
	}

	d.logger.Info("creating database", zap.String("schema_name", d.schema.Name))

	err = client.Ping(d.ctx)
	if err != nil {
		return fmt.Errorf("pinging clickhouse: %w", err)
	}

	if err := client.Do(d.ctx, ch.Query{
		Body: fmt.Sprintf(staticSqlCreatDatabase, d.schema.Name),
	}); err != nil {
		return fmt.Errorf("executing create database sql: %w", err)
	}

	d.logger.Info("database created", zap.String("schema_name", d.schema.Name))

	client, err = d.client()
	if err != nil {
		return fmt.Errorf("getting clickhouse client: %w", err)
	}

	if err := client.Do(d.ctx, ch.Query{
		Body: fmt.Sprintf(staticSqlCreateBlock, d.schema.Name),
	}); err != nil {
		return fmt.Errorf("executing create block sql: %w", err)
	}

	d.logger.Info("block table created", zap.String("schema_name", d.schema.Name))

	if err := client.Do(d.ctx, ch.Query{
		Body: "SET flatten_nested = 1;",
	}); err != nil {
		return fmt.Errorf("executing flatten nested sql: %w", err)
	}

	for _, statement := range d.dialect.CreateTableSql {
		if err := client.Do(d.ctx, ch.Query{
			Body: statement,
		}); err != nil {
			return fmt.Errorf("executing create table sql: %w %q", err, statement)
		}
		d.logger.Info("table created", zap.String("table_name", statement), zap.String("schema_name", d.schema.Name))
	}

	return nil
}

func (d *Database) Insert(table string, values []any) error {
	return d.inserter.insert(table, values)
}

func (d *Database) WalkMessageDescriptorAndInsert(dm *dynamicpb.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent) (time.Duration, error) {
	return d.BaseDatabase.WalkMessageDescriptorAndInsertWithDialect(dm, blockNum, blockTimestamp, parent, d.dialect, d)
}

func (d *Database) BeginTransaction() error {
	return nil
}

func (d *Database) CommitTransaction() error {
	return nil
}

func (d *Database) RollbackTransaction() {
}

func (d *Database) Flush() (time.Duration, error) {
	d.logger.Debug("flushing")

	startFlush := time.Now()
	err := d.inserter.flush(d)
	if err != nil {
		return 0, fmt.Errorf("flushing: %w", err)
	}
	return time.Since(startFlush), nil
}

func (d *Database) GetDialect() sql.Dialect {
	return d.dialect
}

func (d *Database) InsertBlock(blockNum uint64, hash string, timestamp time.Time) error {
	d.logger.Debug("inserting _block_", zap.Uint64("block_num", blockNum), zap.String("block_hash", hash))
	err := d.inserter.insert("_blocks_", []any{blockNum, hash, timestamp, time.Now().UnixNano(), false})
	if err != nil {
		return fmt.Errorf("inserting block %d: %w", blockNum, err)
	}

	return nil
}

func (d *Database) FetchSinkInfo(schemaName string) (*sql.SinkInfo, error) {
	fileName := fmt.Sprintf("%s_schema_hash.txt", schemaName)
	schemaFilePath := path.Join(d.sinkInfoFolder, fileName)
	file, err := os.Open(schemaFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			d.logger.Warn("schema hash file does not exist", zap.String("file_path", schemaFilePath))
			return nil, nil
		}
		return nil, fmt.Errorf("opening schema hash file: %w", err)
	}
	defer file.Close()

	var schemaHash string
	_, err = fmt.Fscanf(file, "%s", &schemaHash)
	if err != nil {
		return nil, fmt.Errorf("reading schema hash from file: %w", err)
	}

	return &sql.SinkInfo{SchemaHash: schemaHash}, nil
}

func (d *Database) StoreSinkInfo(schemaName string, schemaHash string) error {
	fileName := fmt.Sprintf("%s_schema_hash.txt", schemaName)
	schemaFilePath := path.Join(d.sinkInfoFolder, fileName)

	file, err := os.Create(schemaFilePath)
	if err != nil {
		return fmt.Errorf("creating schema hash file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(schemaHash)
	if err != nil {
		return fmt.Errorf("writing schema hash to file: %w", err)
	}

	return nil
}

func (d *Database) UpdateSinkInfoHash(schemaName string, newHash string) error {
	panic("implement me")
}

func (d *Database) FetchCursor() (*sink.Cursor, error) {
	if d.cursorFilePath == "" {
		return nil, fmt.Errorf("cursor file path is not set")
	}

	file, err := os.Open(d.cursorFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening cursor file: %w", err)
	}
	defer file.Close()

	cursorData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading cursor file: %w", err)
	}

	cursor, err := sink.NewCursor(string(cursorData))
	if err != nil {
		return nil, fmt.Errorf("parsing cursor: %w", err)
	}

	return cursor, nil

}

func (d *Database) StoreCursor(cursor *sink.Cursor) error {
	if d.cursorFilePath == "" {
		return fmt.Errorf("cursor file path is not set")
	}

	file, err := os.Create(d.cursorFilePath)
	if err != nil {
		return fmt.Errorf("creating cursor file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(cursor.String())
	if err != nil {
		return fmt.Errorf("writing cursor to file: %w", err)
	}

	return nil
}

func (d *Database) HandleBlocksUndo(lastValidBlockNum uint64) error {
	tables := d.dialect.GetTables()

	// Sort tables in descending order based on their Ordinal field
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Ordinal > tables[j].Ordinal
	})

	client, err := d.client()
	if err != nil {
		return fmt.Errorf("creating clickhouse client: %w", err)
	}

	// local helper with retry and fresh client per attempt
	doWithRetry := func(q string) error {
		retryCount := d.queryRetryCount
		retrySleep := d.queryRetrySleep
		for attempt := 0; ; attempt++ {
			if err := client.Do(d.ctx, ch.Query{Body: q}); err != nil {
				if attempt >= retryCount {
					return fmt.Errorf("executing clickhouse query after %d retries: %w", attempt, err)
				}
				d.logger.Warn("clickhouse query failed, will retry", zap.Int("attempt", attempt+1), zap.Int("max_attempts", retryCount), zap.Error(err))
				time.Sleep(retrySleep)
				fresh, cErr := d.freshClient()
				if cErr != nil {
					return fmt.Errorf("getting fresh client: %w", cErr)
				}
				client = fresh
				continue
			}
			break
		}
		return nil
	}

	err = d.BeginTransaction()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	version := time.Now().UnixNano()

	d.logger.Info("undoing blocks", zap.String("table", "_block_"), zap.Uint64("last_valid_block_num", lastValidBlockNum))
	start := time.Now()
	insertDeleteBlocks := fmt.Sprintf(`
		INSERT INTO %s._blocks_
		SELECT number, hash, timestamp, %d, true
		FROM %s._blocks_ WHERE number > %d
		`, d.schema.Name, version, d.schema.Name, lastValidBlockNum)

	err = doWithRetry(insertDeleteBlocks)
	if err != nil {
		return fmt.Errorf("deleting block from %d: %w", lastValidBlockNum, err)
	}

	//err = client.Do(d.ctx, ch.Query{
	//	Body: fmt.Sprintf("OPTIMIZE TABLE %s._blocks_ FINAL CLEANUP;", d.schema.Name),
	//})
	//if err != nil {
	//	return fmt.Errorf("optimizing table: %w", err)
	//}

	d.logger.Info("undo completed", zap.String("table", "_block_"), zapx.HumanDuration("duration", time.Since(start)))

	for _, table := range tables {
		d.logger.Info("undoing blocks", zap.String("table", table.Name), zap.Uint64("last_valid_block_num", lastValidBlockNum))
		start := time.Now()
		tableFullName := d.dialect.FullTableName(table)
		fields := ""

		if table.ChildOf != nil {
			parentTable, parentFound := d.dialect.TableRegistry[table.ChildOf.ParentTable]
			if !parentFound {
				return fmt.Errorf("parent table %q not found", table.ChildOf.ParentTable)
			}
			fieldFound := false
			for _, parentField := range parentTable.Columns {

				if parentField.Name == table.ChildOf.ParentTableField {
					fields += fmt.Sprintf(", %s", parentField.Name)
					fieldFound = true
					break
				}
			}
			if !fieldFound {
				return fmt.Errorf("field %q not found in table %q", table.ChildOf.ParentTableField, table.ChildOf.ParentTable)
			}
		}

		for _, column := range table.Columns {
			if column.Nested != nil {
				for _, nestedColumn := range column.Nested.Columns {
					fields += fmt.Sprintf(", %s.%s", column.Name, nestedColumn.Name)
				}
			} else {
				fields += fmt.Sprintf(", %s", column.Name)
			}
		}
		query := fmt.Sprintf(`
			INSERT INTO %s
			SELECT %s, %s, %d, true %s
			FROM %s WHERE %s > %d AND _deleted_ != 1
			`, tableFullName, sql.DialectFieldBlockNumber, sql.DialectFieldBlockTimestamp, version, fields, tableFullName, sql.DialectFieldBlockNumber, lastValidBlockNum)

		err := doWithRetry(query)
		if err != nil {
			return fmt.Errorf("deleting block from %d: %w", lastValidBlockNum, err)
		}

		//optimizationStart := time.Now()
		//err = client.Do(d.ctx, ch.Query{
		//	Body: fmt.Sprintf("OPTIMIZE TABLE %s FINAL CLEANUP;", tableFullName),
		//})

		d.logger.Info("undo completed", zap.String("table", table.Name), zapx.HumanDuration("duration", time.Since(start)))
	}
	err = d.CommitTransaction()
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

func (d *Database) Clone() sql.Database {
	base := d.BaseClone()
	d.BaseDatabase = base
	return d
}

func (d *Database) DatabaseHash(schemaName string) (uint64, error) {
	panic("not implemented")
}
