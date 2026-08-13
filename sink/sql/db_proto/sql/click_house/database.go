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
	chproto "github.com/ClickHouse/ch-go/proto"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
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
	spoolOptions    *spool.Options
	spool           *spool.Spool
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

// WithSpool turns on the on-disk spool. It must be called before Open.
func (d *Database) WithSpool(options spool.Options) {
	d.spoolOptions = &options
}

// Open starts the spool, if one is configured.
//
// Rows then land on disk and a background goroutine applies whole segments, so the stream
// stops waiting on ClickHouse. What reaches the server is unchanged: a segment is replayed
// into the same column builders and sent by the same flush, followed by the same cursor
// write.
func (d *Database) Open() error {
	if d.spoolOptions == nil {
		return nil
	}

	created, err := spool.New(d.ctx, *d.spoolOptions, newCHCodec(), newCHApplier(d, d.inserter, d.logger), d.schema.Name, d.logger)
	if err != nil {
		return fmt.Errorf("starting the local spool: %w", err)
	}
	d.spool = created

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

// SwitchToDirectInserts drains the spool and inserts inline from here on. The reason says
// what brought the switch on, since the database cannot tell the chain head from the end
// of a bounded range.
//
// The spool trades freshness for throughput, which is what a backfill wants and the
// opposite of what a sink at the chain head wants, where a block should be queryable when
// it arrives rather than when the segment it lands in is full.
func (d *Database) SwitchToDirectInserts(ctx context.Context, reason string) error {
	if d.spool == nil {
		return nil
	}

	d.logger.Info(reason + ", draining the spool and switching to direct inserts. " +
		"--db-write-* and --spool-* no longer apply from here on")

	if err := d.spool.Close(ctx); err != nil {
		return fmt.Errorf("draining the spool: %w", err)
	}
	d.spool = nil
	d.spoolOptions = nil

	return nil
}

// ApplyConstraints does nothing: ClickHouse has no primary/foreign key constraints to
// apply, which is also why CreateDatabase ignores its useConstraints argument.
func (d *Database) ApplyConstraints() error {
	return nil
}

// EnsureBlockNumberIndexes does nothing: a ClickHouse table's ORDER BY is its index, and
// the sink has no separate one to create.
func (d *Database) EnsureBlockNumberIndexes(context.Context) error {
	return nil
}

// MissingConstraints reports none: ClickHouse has no constraints to be missing.
func (d *Database) MissingConstraints() ([]string, error) {
	return nil, nil
}

// DropConstraints does nothing, for the same reason as ApplyConstraints.
func (d *Database) DropConstraints() error {
	return nil
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

// VerifySchemaCompatibility rejects a database whose tables disagree with the schema the
// current package would create, on the one point the sink cannot paper over: the presence
// of _row_id_.
//
// That column is added exactly when the message carries no 'order_by_fields', so
// annotating a message that had none — or dropping the annotation from one that had them —
// changes the sorting key of a table that already holds rows. CREATE TABLE IF NOT EXISTS
// leaves the old table in place, and the inserts that follow would silently write their
// values into the wrong columns.
func (d *Database) VerifySchemaCompatibility(ctx context.Context) error {
	existing, err := d.rowIDColumnByTable(ctx)
	if err != nil {
		return fmt.Errorf("reading the columns of schema %q: %w", d.schema.Name, err)
	}

	for _, table := range d.dialect.GetTables() {
		found, exists := existing[table.Name]
		if !exists {
			// A table the next CREATE TABLE will add.
			continue
		}

		expected := d.dialect.UseRowIDField(table.Name)
		if found == expected {
			continue
		}

		if expected {
			return fmt.Errorf("table %q in database %q was created from a schema declaring 'order_by_fields' for it, but the package now carries none, so the sink would sort it on (%s, %s) instead. Sink into a fresh database, or restore the annotation",
				table.Name, d.schema.Name, sql.DialectFieldBlockNumber, sql.DialectFieldRowID)
		}

		return fmt.Errorf("table %q in database %q was created without 'order_by_fields' and carries the %s column the sink adds in that case, but the package now declares them. Sink into a fresh database, or drop the annotation",
			table.Name, d.schema.Name, sql.DialectFieldRowID)
	}

	return nil
}

// rowIDColumnByTable reports, for every table of the schema that exists in ClickHouse,
// whether it carries the _row_id_ column. An absent schema yields an empty map rather
// than an error: nothing is set up yet, so there is nothing to disagree with.
//
// It connects to 'default' rather than to the schema, for the same reason CreateDatabase
// does: this runs before the database exists on a first run, and dialing one that is not
// there never comes back — newClient retries a failed dial forever. system.columns is
// global, so which database the connection names does not change the answer.
func (d *Database) rowIDColumnByTable(ctx context.Context) (map[string]bool, error) {
	dsn := d.dsn.Clone()
	dsn.Database = "default"

	client, err := d.clientNoCache(dsn)
	if err != nil {
		return nil, fmt.Errorf("getting clickhouse client: %w", err)
	}
	defer client.Close()

	var (
		tables  chproto.ColStr
		hasRowD chproto.ColUInt64
	)

	out := map[string]bool{}
	query := fmt.Sprintf("SELECT table, sum(name = '%s') AS has_row_id FROM system.columns WHERE database = '%s' GROUP BY table",
		sql.DialectFieldRowID, d.schema.Name)

	if err := client.Do(ctx, ch.Query{
		Body: query,
		Result: chproto.Results{
			{Name: "table", Data: &tables},
			{Name: "has_row_id", Data: &hasRowD},
		},
		OnResult: func(_ context.Context, _ chproto.Block) error {
			for i := 0; i < tables.Rows(); i++ {
				out[tables.Row(i)] = hasRowD[i] > 0
			}

			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("querying system.columns: %w", err)
	}

	return out, nil
}

func (d *Database) Insert(table string, values []any) error {
	if d.spool == nil {
		return d.inserter.insert(table, values)
	}

	if table == sql.DialectTableBlock {
		if blockNum, ok := values[0].(uint64); ok {
			d.spool.RecordBlock(blockNum)
		}
	}

	return d.spool.Insert(table, values)
}

func (d *Database) WalkMessageDescriptorAndInsert(dm protoreflect.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent) (time.Duration, error) {
	return d.BaseDatabase.WalkMessageDescriptorAndInsertWithDialect(dm, blockNum, blockTimestamp, parent, d.dialect, d)
}

func (d *Database) WalkMessageDescriptorAndInsertInto(dm protoreflect.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent, inserter sql.Inserter) (time.Duration, error) {
	return d.BaseDatabase.WalkMessageDescriptorAndInsertWithDialect(dm, blockNum, blockTimestamp, parent, d.dialect, inserter)
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

	// With a spool a flush only seals a segment once it is big enough; the write to
	// ClickHouse happens later, on the applier's goroutine.
	if d.spool != nil {
		if err := d.spool.MaybeSeal(d.ctx); err != nil {
			return 0, fmt.Errorf("sealing: %w", err)
		}

		return time.Since(startFlush), nil
	}

	err := d.inserter.flush(d)
	if err != nil {
		return 0, fmt.Errorf("flushing: %w", err)
	}
	return time.Since(startFlush), nil
}

func (d *Database) GetDialect() sql.Dialect {
	return d.dialect
}

// InsertBlock writes the block row through the same path as every other row.
//
// It must not reach the accumulator directly: with a spool open that instance belongs to
// the applier's goroutine, so appending to it from here is an unsynchronised write to the
// map the applier swaps out on every flush. Going through Insert is also what tells the
// spool which block the rows now being written belong to, without which a segment records
// no block range at all.
func (d *Database) InsertBlock(blockNum uint64, hash string, timestamp time.Time) error {
	d.logger.Debug("inserting _block_", zap.Uint64("block_num", blockNum), zap.String("block_hash", hash))
	err := d.Insert(sql.DialectTableBlock, []any{blockNum, hash, timestamp, time.Now().UnixNano(), false})
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
	// With a spool the cursor belongs to the segment being written: it is what makes that
	// segment resumable, and it must not run ahead of the rows it covers.
	if d.spool != nil {
		d.spool.RecordCursor(cursor.String())

		return nil
	}

	return d.storeCursorFile(cursor)
}

// storeCursorFile writes the cursor where the next run reads it back.
//
// It is what the applier calls once a segment has reached the server. Going through
// StoreCursor would hand the cursor straight back to the spool it just came out of —
// leaving the file untouched for the whole backfill, and stamping an already-applied
// cursor over the newer one on the segment still being written.
func (d *Database) storeCursorFile(cursor *sink.Cursor) error {
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
	// Rows still in the spool would otherwise land after the delete that was supposed to
	// remove them.
	if d.spool != nil {
		if err := d.spool.Drain(d.ctx); err != nil {
			return fmt.Errorf("draining the spool before an undo: %w", err)
		}
	}

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

		// The tombstone only collapses onto the row it deletes if it carries the same
		// sorting key, and _row_id_ is part of it wherever the schema did not declare one.
		if d.dialect.UseRowIDField(table.Name) {
			fields += fmt.Sprintf(", %s", sql.DialectFieldRowID)
		}

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

// Close drains the spool, so blocks held at shutdown reach the server rather than being
// streamed again. Without one there is nothing held: inserts flush inline.
func (d *Database) Close(ctx context.Context) error {
	if d.spool == nil {
		return nil
	}

	return d.spool.Close(ctx)
}

// BufferStats reports what sits between the stream and the server.
func (d *Database) BufferStats() (int64, int64, uint64, bool) {
	if d.spool == nil {
		return 0, 0, 0, false
	}

	return d.spool.BlocksBuffered(), d.spool.BytesOnDisk(), d.spool.AppliedBlock(), true
}

func (d *Database) DatabaseHash(schemaName string) (uint64, error) {
	panic("not implemented")
}
