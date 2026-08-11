package postgres

import (
	"context"
	pgsql "database/sql"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/streamingfast/logging/zapx"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/buffer"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	// maxOpenConnections bounds the sink's own pool: inserts are serialised on one
	// goroutine, and the queries around them are occasional.
	maxOpenConnections = 8
	maxIdleConnections = 2

	// maxCopyConnections bounds the binary COPY pool, driven by a single applier.
	maxCopyConnections = 2
)

type Database struct {
	*sql.BaseDatabase
	db *pgsql.DB
	// pool is only created when the local buffer is enabled: binary COPY needs pgx, the
	// rest of the sink talks to PostgreSQL through database/sql and lib/pq.
	pool           *pgxpool.Pool
	bufferOptions  *buffer.Options
	tx             *pgsql.Tx
	dsn            *db.DSN
	schema         *schema.Schema
	logger         *zap.Logger
	dialect        *DialectPostgres
	inserter       pgInserter
	flusher        pgFlusher
	useConstraints bool
}

// bufferActive reports whether rows are currently being routed to the local buffer.
//
// This is deliberately not "was a buffer configured": schema creation runs before Open
// and does need real transactions, so the distinction is what keeps DDL working.
func (d *Database) bufferActive() bool {
	_, ok := d.inserter.(*localBufferInserter)

	return ok
}

// WithLocalBuffer turns on the on-disk buffer. It must be called before Open.
func (d *Database) WithLocalBuffer(options buffer.Options) {
	d.bufferOptions = &options
}

func NewDatabase(schema *schema.Schema, dsn *db.DSN, moduleOutputType string, rootMessageDescriptor protoreflect.MessageDescriptor, useProtoOptions bool, useConstraints bool, bytesEncoding bytes.Encoding, logger *zap.Logger) (*Database, error) {
	logger = logger.Named("postgres")

	logger.Info("connecting to db", zap.String("host", dsn.Host), zap.Int64("port", dsn.Port), zap.String("database", dsn.Database))
	sqlDB, err := pgsql.Open(dsn.Driver(), dsn.ConnString())
	if err != nil {
		return nil, fmt.Errorf("open db connection: %w", err)
	}

	// The sink writes from one goroutine and queries occasionally around it, so an
	// unbounded pool only ever buys a way to exhaust the server's connection slots.
	sqlDB.SetMaxOpenConns(maxOpenConnections)
	sqlDB.SetMaxIdleConns(maxIdleConnections)

	if reachable, err := isDatabaseReachable(sqlDB); !reachable {
		return nil, fmt.Errorf("database not reachable: %w", err)
	}

	dialect, err := NewDialectPostgres(schema, bytesEncoding, logger)
	if err != nil {
		return nil, fmt.Errorf("creating postgres dialect: %w", err)
	}

	baseDB, err := sql.NewBaseDatabase(moduleOutputType, rootMessageDescriptor, useProtoOptions, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create base database: %w", err)
	}
	database := &Database{
		db:             sqlDB,
		dsn:            dsn,
		schema:         schema,
		useConstraints: useConstraints,
		BaseDatabase:   baseDB,
		dialect:        dialect,
		logger:         logger,
	}

	return database, nil
}

func (d *Database) Open() error {
	if d.bufferOptions != nil {
		ctx := context.Background()

		poolConfig, err := pgxpool.ParseConfig(d.dsn.ConnString())
		if err != nil {
			return fmt.Errorf("parsing the connection string for binary COPY: %w", err)
		}
		// One applier goroutine COPYs one segment at a time, so the pgx default of four
		// connections per core is a fleet of idle connections against the server.
		poolConfig.MaxConns = maxCopyConnections

		pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			return fmt.Errorf("connecting to the database for binary COPY: %w", err)
		}
		d.pool = pool

		inserter, err := newLocalBufferInserter(ctx, d, *d.bufferOptions, d.logger)
		if err != nil {
			return fmt.Errorf("starting the local buffer: %w", err)
		}
		d.inserter = inserter
		d.flusher = inserter

		return nil
	}

	return d.openDirectInserter()
}

// openDirectInserter installs the inserter that writes to the database itself: one
// prepared INSERT per row when constraints have to be respected row by row, a multi-row
// INSERT built at flush otherwise.
func (d *Database) openDirectInserter() error {
	if d.useConstraints {
		inserter, err := NewRowInserter(d.logger)
		if err != nil {
			return fmt.Errorf("creating row inserter: %w", err)
		}
		if err := inserter.init(d); err != nil {
			return fmt.Errorf("initializing row inserter: %w", err)
		}
		d.inserter = inserter
		d.flusher = inserter

		return nil
	}

	inserter, err := NewAccumulatorInserter(d.logger)
	if err != nil {
		return fmt.Errorf("creating accumulator inserter: %w", err)
	}
	if err := inserter.init(d); err != nil {
		return fmt.Errorf("initializing accumulator inserter: %w", err)
	}
	d.inserter = inserter
	d.flusher = inserter

	return nil
}

// SwitchToDirectInserts drains the local buffer and inserts straight into the database
// from here on.
//
// The buffer trades freshness for throughput: rows sit on disk until a segment fills,
// which is what a backfill wants and the opposite of what a sink at the chain head wants,
// where a block should be queryable when it arrives rather than when the segment it
// happens to land in is full. Reorgs are also cheaper to undo out of a table than out of
// a segment that has not been applied yet.
//
// Everything buffered is applied before the switch, so no row is left behind, and the
// buffer is closed for good — a stream that has reached the head does not go back.
func (d *Database) SwitchToDirectInserts(ctx context.Context) error {
	inserter, ok := d.inserter.(*localBufferInserter)
	if !ok {
		return nil
	}

	d.logger.Info("stream reached the chain head, draining the local buffer and switching to direct inserts")

	if err := inserter.close(ctx); err != nil {
		return fmt.Errorf("draining the local buffer: %w", err)
	}

	if err := d.openDirectInserter(); err != nil {
		return fmt.Errorf("switching to direct inserts: %w", err)
	}

	// bufferActive() reads the inserter, so transactions resume from here; the options
	// go with it to keep the two from disagreeing.
	d.bufferOptions = nil

	if d.pool != nil {
		d.pool.Close()
		d.pool = nil
	}

	return nil
}

func (d *Database) GetDialect() sql.Dialect {
	return d.dialect
}

func (d *Database) CreateDatabase(useConstraints bool) error {
	err := d.createDatabase()
	if err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	if useConstraints {
		err = d.applyConstraints()
		if err != nil {
			return fmt.Errorf("applying constraints: %w", err)
		}
	}

	return nil
}

func (d *Database) createDatabase() error {
	staticSql := fmt.Sprintf(postgresStaticSql, d.schema.Name, d.schema.Name, d.schema.Name, d.schema.Name)
	_, err := d.tx.Exec(staticSql)
	if err != nil {
		return fmt.Errorf("executing static staticSql: %w\n%s", err, staticSql)
	}

	for _, statement := range d.dialect.CreateTableSql {
		d.logger.Info("executing create statement", zap.String("sql", statement))
		_, err := d.tx.Exec(statement)
		if err != nil {
			return fmt.Errorf("executing create statement: %w %s", err, statement)
		}
	}
	return nil
}

// ApplyConstraints puts the dialect's constraints on a schema that already exists,
// leaving the ones already there alone.
//
// It is what --with-constraints does to a database first synced without them: the sink
// info hash cannot tell those two apart, since it is computed over the DDL the dialect
// would emit, constraints included, either way.
//
// The caller owns the transaction. On a populated database this is not quick — every
// index has to be built and every foreign key validated, with the table locked meanwhile.
func (d *Database) ApplyConstraints() error {
	return d.applyConstraints()
}

func (d *Database) applyConstraints() error {
	startAt := time.Now()

	existing, err := d.existingConstraintNames()
	if err != nil {
		return err
	}

	apply := func(kind string, constraints []*sql.Constraint) error {
		for _, constraint := range constraints {
			if name := constraintName(constraint.Sql); name != "" && existing[name] {
				d.logger.Debug("constraint already in place, skipping", zap.String("constraint", name))
				continue
			}

			d.logger.Info("executing "+kind+" statement", zap.String("sql", constraint.Sql))
			if _, err := d.tx.Exec(constraint.Sql); err != nil {
				return fmt.Errorf("executing %s statement: %w %s", kind, err, constraint.Sql)
			}
		}

		return nil
	}

	if err := apply("pk", d.dialect.PrimaryKeySql); err != nil {
		return err
	}
	if err := apply("unique", d.dialect.UniqueConstraintSql); err != nil {
		return err
	}
	if err := apply("fk constraint", d.dialect.ForeignKeySql); err != nil {
		return err
	}

	d.logger.Info("applying constraints", zapx.HumanDuration("duration", time.Since(startAt)))
	return nil
}

// existingConstraintNames is what makes applying constraints re-runnable: PostgreSQL has
// no ADD CONSTRAINT IF NOT EXISTS, and the errors it raises instead differ by kind — a
// second primary key is `multiple primary keys` rather than a duplicate name — so the
// check has to happen before the statement rather than around it.
func (d *Database) existingConstraintNames() (map[string]bool, error) {
	rows, err := d.tx.Query(`
		SELECT c.conname
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE n.nspname = $1`, d.schema.Name)
	if err != nil {
		return nil, fmt.Errorf("listing the existing constraints of schema %q: %w", d.schema.Name, err)
	}
	defer rows.Close()

	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning constraint name: %w", err)
		}
		names[name] = true
	}

	return names, rows.Err()
}

// constraintNamePattern pulls the name out of the dialect's own DDL. Every constraint it
// emits is named, so an empty result means the statement is not one of ours and is left
// to the server to accept or reject.
var constraintNamePattern = regexp.MustCompile(`(?i)add\s+constraint\s+"?([a-z0-9_]+)"?`)

func constraintName(statement string) string {
	match := constraintNamePattern.FindStringSubmatch(statement)
	if len(match) != 2 {
		return ""
	}

	return match[1]
}

func (d *Database) BeginTransaction() (err error) {
	if d.bufferActive() {
		// The buffer owns its transactions: one per segment, in the applier goroutine.
		// Holding one here would pin a connection for the whole buffering window and
		// would not cover the writes anyway.
		return nil
	}

	d.tx, err = d.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	return nil
}

func (d *Database) CommitTransaction() (err error) {
	if d.bufferActive() {
		return nil
	}

	err = d.tx.Commit()
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	d.tx = nil
	return nil
}

func (d *Database) RollbackTransaction() {
	if d.bufferActive() {
		return
	}

	err := d.tx.Rollback()
	if err != nil {
		panic("RollbackTransaction failed: " + err.Error())
	}
}

func (d *Database) wrapInsertStatement(stmt *pgsql.Stmt) *pgsql.Stmt {
	if d.tx != nil {
		stmt = d.tx.Stmt(stmt)
	}
	return stmt
}

func (d *Database) Insert(table string, values []any) error {
	return d.inserter.insert(table, values, d)
}

func (d *Database) WalkMessageDescriptorAndInsert(dm *dynamicpb.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent) (time.Duration, error) {
	return d.WalkMessageDescriptorAndInsertWithDialect(dm, blockNum, blockTimestamp, parent, d.dialect, d)
}

func (d *Database) WalkMessageDescriptorAndInsertInto(dm *dynamicpb.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent, inserter sql.Inserter) (time.Duration, error) {
	return d.WalkMessageDescriptorAndInsertWithDialect(dm, blockNum, blockTimestamp, parent, d.dialect, inserter)
}

func (d *Database) InsertBlock(blockNum uint64, hash string, timestamp time.Time) error {
	d.logger.Debug("inserting _blocks_", zap.Uint64("block_num", blockNum), zap.String("block_hash", hash))
	err := d.inserter.insert("_blocks_", []any{blockNum, hash, timestamp}, d)
	if err != nil {
		return fmt.Errorf("inserting block %d: %w", blockNum, err)
	}

	return nil
}

// Close drains the local buffer, if one is in use, so the blocks buffered at shutdown
// reach the database rather than being streamed again on the next run.
// Close drains a local buffer, if one is in use, and then releases the connections. It
// is called once the stream is done with the database, so holding the pools open past it
// only occupies connection slots on the server.
func (d *Database) Close(ctx context.Context) error {
	var err error
	if inserter, ok := d.inserter.(*localBufferInserter); ok {
		err = inserter.close(ctx)
	}

	if d.pool != nil {
		d.pool.Close()
		d.pool = nil
	}

	if closeErr := d.db.Close(); closeErr != nil && err == nil {
		err = fmt.Errorf("closing the database connections: %w", closeErr)
	}

	return err
}

// BufferStats reports what the local buffer is holding, for the progress line.
func (d *Database) BufferStats() (blocks int64, bytes int64, appliedBlock uint64, enabled bool) {
	inserter, ok := d.inserter.(*localBufferInserter)
	if !ok {
		return 0, 0, 0, false
	}

	return inserter.buffer.BlocksBuffered(), inserter.buffer.BytesOnDisk(), inserter.buffer.AppliedBlock(), true
}

func (d *Database) Flush() (time.Duration, error) {
	startFlush := time.Now()
	err := d.flusher.flush(d)
	if err != nil {
		return 0, fmt.Errorf("flushing: %w", err)
	}
	return time.Since(startFlush), nil
}

func (d *Database) FetchSinkInfo(schemaName string) (*sql.SinkInfo, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = '%s' AND table_name = '_sink_info_')", schemaName)

	var exist bool
	err := d.db.QueryRow(query).Scan(&exist)
	if err != nil {
		return nil, fmt.Errorf("checking if sync_info table exists: %w", err)
	}
	if !exist {
		return nil, nil
	}

	out := &sql.SinkInfo{}

	err = d.db.QueryRow(fmt.Sprintf("SELECT schema_hash FROM %s._sink_info_", d.schema.Name)).Scan(&out.SchemaHash)
	if err != nil {
		return nil, fmt.Errorf("fetching sync info: %w", err)
	}
	return out, nil

}

func (d *Database) StoreSinkInfo(schemaName string, schemaHash string) error {
	_, err := d.tx.Exec(fmt.Sprintf("INSERT INTO %s._sink_info_ (schema_hash) VALUES ($1)", schemaName), schemaHash)
	if err != nil {
		return fmt.Errorf("storing schema hash: %w", err)
	}
	return nil
}

func (d *Database) UpdateSinkInfoHash(schemaName string, newHash string) error {
	_, err := d.tx.Exec(fmt.Sprintf("UPDATE %s._sink_info_ SET schema_hash = $1", schemaName), newHash)
	if err != nil {
		return fmt.Errorf("updating schema hash: %w", err)
	}
	return nil
}

func (d *Database) FetchCursor() (*sink.Cursor, error) {
	query := fmt.Sprintf("SELECT cursor FROM %s WHERE name = $1", tableName(d.schema.Name, "_cursor_"))

	rows, err := d.db.Query(query, "cursor")
	if err != nil {
		return nil, fmt.Errorf("selecting cursor: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var cursor string
		err = rows.Scan(&cursor)

		return sink.NewCursor(cursor)
	}
	return nil, nil
}

func (d *Database) StoreCursor(cursor *sink.Cursor) error {
	err := d.inserter.insert("_cursor_", []any{"cursor", cursor.String()}, d)
	if err != nil {
		return fmt.Errorf("inserting cursor: %w", err)
	}

	return err
}

// HandleBlocksUndo removes everything a reorg invalidated: every entity row above the
// last valid block, then the block rows themselves.
//
// The deletes are explicit rather than left to `fk_block ... ON DELETE CASCADE`, because
// that foreign key only exists with --with-constraints. Deleting just the block rows, as
// this used to, silently orphaned every entity row of the undone blocks on a schema
// without constraints — and that is now the default. Every table carries
// `_block_number_`, so the same delete works either way, and the descending Ordinal
// visits children before parents so a foreign key never blocks its own cleanup.
func (d *Database) HandleBlocksUndo(lastValidBlockNum uint64) (err error) {
	// Rows for the undone blocks may still be sitting in the buffer, on their way to a
	// COPY; applying them after the delete would resurrect exactly what it removed.
	if inserter, ok := d.inserter.(*localBufferInserter); ok {
		if err := inserter.buffer.Drain(context.Background()); err != nil {
			return fmt.Errorf("draining the local buffer before an undo: %w", err)
		}
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("HandleBlocksUndo beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			e := tx.Rollback()
			if e != nil {
				err = fmt.Errorf("HandleBlocksUndo rolling back transaction: %w", e)
			}
			err = fmt.Errorf("HandleBlocksUndo processing entity: %w", err)

			return
		}
		err = tx.Commit()
	}()

	d.logger.Info("undoing blocks", zap.Uint64("last_valid_block_num", lastValidBlockNum))
	startAt := time.Now()

	tables := d.dialect.GetTables()
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Ordinal > tables[j].Ordinal
	})

	var rowsAffected int64
	for _, table := range tables {
		query := fmt.Sprintf(`DELETE FROM %s WHERE %s > $1`, tableName(d.schema.Name, table.Name), sql.DialectFieldBlockNumber)
		result, err := tx.Exec(query, lastValidBlockNum)
		if err != nil {
			return fmt.Errorf("deleting rows of %q from %d: %w", table.Name, lastValidBlockNum, err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("fetching rows affected: %w", err)
		}
		rowsAffected += affected
	}

	query := fmt.Sprintf(`DELETE FROM %s._blocks_ WHERE "number" > $1`, d.schema.Name)
	result, err := tx.Exec(query, lastValidBlockNum)
	if err != nil {
		return fmt.Errorf("deleting block from %d: %w", lastValidBlockNum, err)
	}
	blocksAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("fetching rows affected: %w", err)
	}

	d.logger.Info("undo completed",
		zap.Int64("row_affected", rowsAffected),
		zap.Int64("block_affected", blocksAffected),
		zapx.HumanDuration("duration", time.Since(startAt)))

	return nil
}

func (d *Database) DatabaseHash(schemaName string) (uint64, error) {
	query := `
SELECT
    c.table_name,
    c.column_name,
    c.is_nullable,
    c.data_type,
    c.character_maximum_length,
    c.numeric_precision,
    c.numeric_precision_radix,
    c.numeric_scale,
    c.datetime_precision,
    c.interval_precision,
    c.is_generated,
    c.is_updatable,
    tc.constraint_name,
    tc.table_name,
    tc.constraint_type,
    kcu.column_name,
    kcu.table_name,
    kcu.column_name,
    ccu.constraint_name,
    ccu.table_name,
    ccu.column_name
FROM
    information_schema.columns c
        LEFT JOIN
    information_schema.constraint_column_usage ccu
    ON c.table_name = ccu.table_name
        AND c.column_name = ccu.column_name
        AND c.table_schema = ccu.table_schema
        LEFT JOIN
    information_schema.key_column_usage kcu
    ON ccu.constraint_name = kcu.constraint_name
        AND c.table_schema = kcu.table_schema
        LEFT JOIN
    information_schema.table_constraints tc
    ON kcu.constraint_name = tc.constraint_name
        AND kcu.table_schema = tc.table_schema
WHERE
    c.table_schema = '%s'
ORDER BY
    c.table_name,
    c.column_name,
    tc.table_name,
    tc.constraint_name,
    kcu.table_name,
    kcu.column_name,
    kcu.constraint_name;
`

	query = fmt.Sprintf(query, schemaName)

	rows, err := d.db.Query(query)
	if err != nil {
		return 0, fmt.Errorf("executing query to compute schema hash: %w", err)
	}
	defer rows.Close()

	h := fnv.New64a()
	columns, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("fetching columns for hashing: %w", err)
	}

	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		err = rows.Scan(valuePtrs...)
		if err != nil {
			return 0, fmt.Errorf("scanning row for hashing: %w", err)
		}

		for _, val := range values {
			var str string
			if val != nil {
				str = fmt.Sprintf("%v", val)
			}
			_, err = h.Write([]byte(str))
			if err != nil {
				return 0, fmt.Errorf("hashing value %q: %w", str, err)
			}
		}
	}

	if err = rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating rows: %w", err)
	}

	return h.Sum64(), nil
}

func isDatabaseReachable(db *pgsql.DB) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := db.PingContext(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}
