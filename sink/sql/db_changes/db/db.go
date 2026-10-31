package db

import (
	"cmp"
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/netip"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/streamingfast/logging"
	sink "github.com/streamingfast/substreams/sink"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Make the typing a bit easier
type OrderedMap[K comparable, V any] struct {
	*orderedmap.OrderedMap[K, V]
}

func NewOrderedMap[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{OrderedMap: orderedmap.New[K, V]()}
}

type SystemTableError struct {
	error
}

type Loader struct {
	*sql.DB

	entries      *OrderedMap[string, *OrderedMap[string, *Operation]]
	entriesCount uint64
	tables       map[string]*TableInfo
	cursorTable  *TableInfo

	handleReorgs            bool
	batchBlockFlushInterval int
	batchRowFlushInterval   int
	liveBlockFlushInterval  int
	moduleMismatchMode      OnModuleHashMismatch

	dialect Dialect

	logger *zap.Logger
	tracer logging.Tracer

	testTx *TestTx // used for testing: if non-nil, 'loader.BeginTx()' will return this object instead of a real *sql.Tx
	dsn    *DSN

	batchOrdinal uint64 // Counter for ordinals within the current batch, resets on flush

	// detached holds the changes of a background flush, apart from those being added.
	detached *OrderedMap[string, *OrderedMap[string, *Operation]]
	// currentBlockNum is the block the changes being added come from (see SetCurrentBlock).
	currentBlockNum uint64
	// activeModuleHash and activeCursor are those of the running flush or revert.
	activeModuleHash string
	activeCursor     *sink.Cursor
	// reverting is set while Revert runs (see ClickhouseDialect.resolveUncommitted).
	reverting bool

	// expectedCursor is the stored cursor of this sink's module as this loader last read or wrote
	// it, nil until SetExpectedCursor. Flushes and reverts first check the stored cursor still has this
	// value (see checkCursor).
	expectedCursor *string

	// writerConn, once the writer lock is held (see AcquireWriterLock), is the connection holding
	// it. Cursor reads and every write go through it, so the lock covers them.
	writerConn      *sql.Conn
	clickhouseLease *clickhouseLease
	// writerConnMu is held while a transaction or a result set is open on writerConn, so the
	// keepalive (see KeepWriterSessionAlive) never runs inside a transaction or between the rows of
	// a query (a connection refuses a query while another one's rows are being read).
	writerConnMu sync.Mutex
}

func NewLoader(
	dsn *DSN,
	cursorTableName string, historyTableName string, clickhouseCluster string,
	batchBlockFlushInterval int,
	batchRowFlushInterval int,
	liveBlockFlushInterval int,
	OnModuleHashMismatch string,
	handleReorgs *bool,
	logger *zap.Logger,
	tracer logging.Tracer,
) (*Loader, error) {

	// Validate ClickHouse is not using HTTP protocol ports
	if dsn.Driver() == "clickhouse" {
		if dsn.Port == 8123 || dsn.Port == 8443 {
			return nil, fmt.Errorf("ClickHouse HTTP protocol (port %d) is not supported. Please use the native TCP protocol on port 9000 or 9440", dsn.Port)
		}
	}

	sqlDB, err := OpenDB(dsn)
	if err != nil {
		return nil, fmt.Errorf("open db connection: %w", err)
	}

	dialect, err := newDialect(sqlDB.Driver(), dsn, cursorTableName, historyTableName, clickhouseCluster)
	if err != nil {
		return nil, fmt.Errorf("get dialect: %w", err)
	}

	moduleMismatchMode, err := ParseOnModuleHashMismatch(OnModuleHashMismatch)
	if err != nil {
		return nil, fmt.Errorf("parse on module hash mismatch: %w", err)
	}

	l := &Loader{
		DB:                      sqlDB,
		dsn:                     dsn,
		entries:                 NewOrderedMap[string, *OrderedMap[string, *Operation]](),
		tables:                  map[string]*TableInfo{},
		batchBlockFlushInterval: batchBlockFlushInterval,
		batchRowFlushInterval:   batchRowFlushInterval,
		liveBlockFlushInterval:  liveBlockFlushInterval,
		moduleMismatchMode:      moduleMismatchMode,
		dialect:                 dialect,
		logger:                  logger,
		tracer:                  tracer,
	}

	if handleReorgs == nil {
		// automatic detection
		l.handleReorgs = l.dialect.Capabilities().Reorgs
	} else {
		l.handleReorgs = *handleReorgs
	}

	if l.handleReorgs && !l.dialect.Capabilities().Reorgs {
		return nil, fmt.Errorf("driver %s does not support reorg handling. You must use set a non-zero undo-buffer-size", sqlDB.Driver())
	}

	logger.Info("created new DB loader",
		zap.Int("batch_block_flush_interval", batchBlockFlushInterval),
		zap.Int("batch_row_flush_interval", batchRowFlushInterval),
		zap.Int("live_block_flush_interval", liveBlockFlushInterval),
		zap.Stringer("on_module_hash_mismatch", moduleMismatchMode),
		zap.Bool("handle_reorgs", l.handleReorgs),
		zap.String("dialect", fmt.Sprintf("%T", l.dialect)),
	)

	return l, nil
}

// OpenDB opens the database.
func OpenDB(dsn *DSN) (*sql.DB, error) {
	if dsn.Driver() != "postgres" {
		return sql.Open(dsn.Driver(), dsn.ConnString())
	}
	config, err := PostgresConnConfig(dsn)
	if err != nil {
		return nil, err
	}
	return stdlib.OpenDB(*config), nil
}

// PostgresConnConfig is how the sink connects to PostgreSQL:
//
//   - TLS is required unless the DSN or PGSSLMODE says otherwise, or a service turns it off, never
//     falling back to plain;
//   - statements are sent as plain SQL rather than prepared, as their values are in their text, in a
//     session set up to read them the same whatever the server's or role's defaults;
//   - a cancelled context sends the server a cancel request, leaving the connection and its session
//     usable, which the writer lock lives in.
func PostgresConnConfig(dsn *DSN) (*pgx.ConnConfig, error) {
	connString := dsn.ConnString()
	config, err := pgx.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	if dsn.Options.Get("sslmode") == "" && os.Getenv("PGSSLMODE") == "" && postgresFallsBackToPlain(config) {
		config, err = pgx.ParseConfig(connString + " sslmode=require")
		if err != nil {
			return nil, err
		}
	}
	if err := applyLibpqOptions(dsn, config); err != nil {
		return nil, err
	}
	config.RuntimeParams["client_encoding"] = "UTF8"
	config.RuntimeParams["datestyle"] = "ISO, MDY"
	config.RuntimeParams["standard_conforming_strings"] = "on"
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	config.BuildContextWatcherHandler = func(conn *pgconn.PgConn) ctxwatch.Handler {
		return &pgconn.CancelRequestContextWatcherHandler{Conn: conn, DeadlineDelay: postgresCancelDeadline}
	}
	return config, nil
}

// postgresFallsBackToPlain reports whether config is sslmode=prefer, which pgx defaults to: each host
// tried with TLS, then without.
func postgresFallsBackToPlain(config *pgx.ConnConfig) bool {
	previousTLS, previousHost := config.TLSConfig, config.Host
	for _, fallback := range config.Fallbacks {
		if previousTLS != nil && fallback.TLSConfig == nil && fallback.Host == previousHost {
			return true
		}
		previousTLS, previousHost = fallback.TLSConfig, fallback.Host
	}
	return false
}

// applyLibpqOptions handles the DSN options lib/pq took and pgx doesn't know, which it would send
// the server as settings to be refused. Those it can't do are an error rather than ignored.
func applyLibpqOptions(dsn *DSN, config *pgx.ConnConfig) error {
	take := func(option string) string {
		value := config.RuntimeParams[option]
		delete(config.RuntimeParams, option)
		return value
	}

	// How lib/pq sent values, or in the others' case read and never used.
	take("binary_parameters")
	take("disable_prepared_binary_result")
	take("tz")
	take("geqo")
	// pgx sets the session's time zone from PGTZ, lib/pq left it to the server.
	if tz := os.Getenv("PGTZ"); tz != "" && dsn.Options.Get("timezone") == "" && config.RuntimeParams["timezone"] == tz {
		take("timezone")
	}

	if name := take("fallback_application_name"); name != "" {
		if _, set := config.RuntimeParams["application_name"]; !set {
			config.RuntimeParams["application_name"] = name
		}
	}

	minVersion, err := postgresTLSVersion("ssl_min_protocol_version", cmp.Or(take("ssl_min_protocol_version"), os.Getenv("SSLPGMINPROTOCOLVERSION")))
	if err != nil {
		return err
	}
	maxVersion, err := postgresTLSVersion("ssl_max_protocol_version", cmp.Or(take("ssl_max_protocol_version"), os.Getenv("SSLPGMAXPROTOCOLVERSION")))
	if err != nil {
		return err
	}
	tlsConfigs := []*tls.Config{config.TLSConfig}
	for _, fallback := range config.Fallbacks {
		tlsConfigs = append(tlsConfigs, fallback.TLSConfig)
	}
	for _, tlsConfig := range tlsConfigs {
		if tlsConfig != nil {
			tlsConfig.MinVersion = minVersion
			tlsConfig.MaxVersion = maxVersion
		}
	}

	if hostaddr := cmp.Or(take("hostaddr"), os.Getenv("PGHOSTADDR")); hostaddr != "" {
		addr, err := netip.ParseAddr(hostaddr)
		if err != nil {
			return fmt.Errorf("hostaddr %q is not supported, only a single address: %w", hostaddr, err)
		}
		if network, _ := pgconn.NetworkAddress(config.Host, config.Port); network == "unix" {
			return fmt.Errorf("hostaddr is not supported with a socket as host")
		}
		for _, fallback := range config.Fallbacks {
			if fallback.Host != config.Host {
				return fmt.Errorf("hostaddr is not supported with several hosts")
			}
		}
		// The host name stays what TLS names, and checks the server against with verify-full.
		config.LookupFunc = func(context.Context, string) ([]string, error) {
			return []string{addr.String()}, nil
		}
	}

	switch value := take("sslinline"); value {
	case "", "0", "f", "F", "false", "FALSE", "False", "no", "off":
	default:
		return fmt.Errorf("the sslinline option is not supported, use files for sslcert, sslkey and sslrootcert")
	}
	if value := cmp.Or(take("load_balance_hosts"), os.Getenv("PGLOADBALANCEHOSTS")); value != "" && value != "disable" {
		return fmt.Errorf("load_balance_hosts=%s is not supported", value)
	}
	return nil
}

// postgresTLSVersion gives the TLS version of a lib/pq option's value, 0 when it isn't set.
func postgresTLSVersion(option, value string) (uint16, error) {
	switch value {
	case "":
		return 0, nil
	case "TLSv1.0":
		return tls.VersionTLS10, nil
	case "TLSv1.1":
		return tls.VersionTLS11, nil
	case "TLSv1.2":
		return tls.VersionTLS12, nil
	case "TLSv1.3":
		return tls.VersionTLS13, nil
	}
	return 0, fmt.Errorf("invalid %s %q", option, value)
}

// postgresCancelDeadline is how long a cancelled query gets to stop before its connection is closed.
const postgresCancelDeadline = 5 * time.Second

// postgresScanType is how a column's values are written (see normalizeValueType). A column of any
// other type than these takes its value as the module gives it, whatever the driver scans it to.
func postgresScanType(column *sql.ColumnType) reflect.Type {
	switch column.DatabaseTypeName() {
	case "INT2", "INT4", "INT8", "FLOAT4", "FLOAT8", "BOOL", "BYTEA", "TEXT", "VARCHAR", "BIT", "VARBIT":
		return column.ScanType()
	case "TIMESTAMP", "TIMESTAMPTZ", "DATE", "TIME", "TIMETZ", "1266": // the driver names timetz by its OID
		return reflectTypeTime
	}
	return reflect.TypeFor[any]()
}

func newDialect(driver driver.Driver, dsn *DSN, cursorTableName string, historyTableName string, clickHouseClusterName string) (Dialect, error) {
	driverType := fmt.Sprintf("%T", driver)
	switch driverType {
	case "*stdlib.Driver":
		return NewPostgresDialect(dsn.Schema(), cursorTableName, historyTableName), nil
	case "*clickhouse.stdDriver":
		return NewClickhouseDialect(dsn.Schema(), cursorTableName, clickHouseClusterName), nil
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driverType)
	}
}

type Tx interface {
	Rollback() error
	Commit() error
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (l *Loader) Begin() (Tx, error) {
	return l.BeginTx(context.Background(), nil)
}

func (l *Loader) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	if l.testTx != nil {
		return l.testTx, nil
	}
	if l.writerConn != nil {
		l.writerConnMu.Lock()
		tx, err := l.writerConn.BeginTx(ctx, opts)
		if err != nil {
			l.writerConnMu.Unlock()
			return nil, err
		}
		return &connTx{Tx: tx, conn: l.writerConn, release: sync.OnceFunc(l.writerConnMu.Unlock)}, nil
	}
	if l.dsn.Driver() == "postgres" {
		conn, err := l.Conn(ctx)
		if err != nil {
			return nil, err
		}
		tx, err := conn.BeginTx(ctx, opts)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return &connTx{Tx: tx, conn: conn, release: sync.OnceFunc(func() { _ = conn.Close() })}, nil
	}
	return l.DB.BeginTx(ctx, opts)
}

// connTx is a PostgreSQL transaction on a known connection (the writer lock's or one of its own), to
// reach the driver's pipeline on it. It releases the connection once it ends.
type connTx struct {
	*sql.Tx
	conn    *sql.Conn
	release func()
}

func (t *connTx) Commit() error {
	defer t.release()
	return t.Tx.Commit()
}

func (t *connTx) Rollback() error {
	defer t.release()
	return t.Tx.Rollback()
}

// sendStatements runs statements in order in one round trip, as a pipeline: each is a statement of
// its own for the server (timestamp, timeout, errors). It gives how many completed before an error.
func (t *connTx) sendStatements(ctx context.Context, statements []string) (completed int, err error) {
	err = t.conn.Raw(func(driverConn any) error {
		batch := &pgconn.Batch{}
		for _, statement := range statements {
			batch.ExecParams(statement, nil, nil, nil, nil)
		}
		results, err := driverConn.(*stdlib.Conn).Conn().PgConn().ExecBatch(ctx, batch).ReadAll()
		completed = len(results)
		// A statement that fails after describing its rows has a result of its own, holding the error.
		if completed > 0 && results[completed-1].Err != nil {
			completed--
		}
		return err
	})
	return completed, err
}

type queryExecutor interface {
	sqlExecutor
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// execer returns where statements outside a transaction run: the connection holding the writer
// lock if any, the pool otherwise.
func (l *Loader) execer() sqlExecutor {
	return l.queryer()
}

func (l *Loader) queryer() queryExecutor {
	if l.writerConn != nil {
		return l.writerConn
	}
	return l.DB
}

// Close releases the writer lock, if held, and closes the database.
func (l *Loader) Close() error {
	l.ReleaseWriterLock()
	return l.DB.Close()
}

func (l *Loader) BatchBlockFlushInterval() int {
	return l.batchBlockFlushInterval
}

func (l *Loader) LiveBlockFlushInterval() int {
	return l.liveBlockFlushInterval
}

func (l *Loader) FlushNeeded() bool {
	totalRows := 0
	// todo keep a running count when inserting/deleting rows directly
	for pair := l.entries.Oldest(); pair != nil; pair = pair.Next() {
		totalRows += pair.Value.Len()
	}
	return totalRows > l.batchRowFlushInterval
}

// HasPendingEntries reports whether any change is waiting to be flushed.
func (l *Loader) HasPendingEntries() bool {
	for pair := l.entries.Oldest(); pair != nil; pair = pair.Next() {
		if pair.Value.Len() > 0 {
			return true
		}
	}
	return false
}

// getTablesFromSchema returns table information similar to schema.Tables()
// but only inspects tables in the specified schema to avoid issues with database extensions
func (l *Loader) getTablesFromSchema(schemaName string) (map[[2]string][]*sql.ColumnType, error) {
	// Use dialect-specific method to get tables
	tables, err := l.dialect.GetTablesInSchema(l.DB, schemaName)
	if err != nil {
		return nil, fmt.Errorf("getting tables from schema: %w", err)
	}

	result := make(map[[2]string][]*sql.ColumnType)

	for _, table := range tables {
		schemaName, tableName := table[0], table[1]

		// Get column information for this table
		columns, err := l.dialect.GetTableColumns(l.DB, schemaName, tableName)
		if err != nil {
			l.logger.Warn("failed to get columns for table, skipping",
				zap.String("schema", schemaName),
				zap.String("table", tableName),
				zap.Error(err),
			)
			continue
		}

		key := [2]string{schemaName, tableName}
		result[key] = columns
	}

	return result, nil
}

func (l *Loader) LoadTables(schemaName string, cursorTableName string, historyTableName string) error {
	schemaTables, err := l.getTablesFromSchema(schemaName)
	if err != nil {
		return fmt.Errorf("retrieving table and schemaName: %w", err)
	}

	seenCursorTable := false
	seenHistoryTable := false
	for schemaTableName, columns := range schemaTables {
		tableName := schemaTableName[1]
		l.logger.Debug("processing schemaName's table",
			zap.String("schema_name", schemaName),
			zap.String("table_name", tableName),
		)

		if schemaTableName[0] != schemaName {
			continue
		}

		if tableName == cursorTableName {
			if err := l.validateCursorTables(columns, schemaName, cursorTableName); err != nil {
				return fmt.Errorf("invalid cursors table: %w", err)
			}

			seenCursorTable = true
		}
		if tableName == historyTableName {
			seenHistoryTable = true
		}

		columnByName := make(map[string]*ColumnInfo, len(columns))
		for _, f := range columns {
			scanType := f.ScanType()
			if _, isPostgres := l.dialect.(*PostgresDialect); isPostgres {
				scanType = postgresScanType(f)
			}
			columnByName[f.Name()] = &ColumnInfo{
				name:             f.Name(),
				escapedName:      EscapeIdentifier(f.Name()),
				databaseTypeName: f.DatabaseTypeName(),
				scanType:         scanType,
			}
		}

		key, err := l.dialect.GetPrimaryKey(l.DB, schemaName, tableName)
		if err != nil {
			return fmt.Errorf("get primary key: %w", err)
		}

		l.tables[tableName], err = NewTableInfo(schemaName, tableName, key, columnByName)
		if err != nil {
			return fmt.Errorf("invalid table: %w", err)
		}
	}

	if !seenCursorTable {
		return &SystemTableError{fmt.Errorf(`%s.%s table is not found`, EscapeIdentifier(schemaName), cursorTableName)}
	}
	if l.handleReorgs && !seenHistoryTable {
		return &SystemTableError{fmt.Errorf("%s.%s table is not found and reorgs handling is enabled", EscapeIdentifier(schemaName), historyTableName)}
	}

	l.cursorTable = l.tables[cursorTableName]

	return nil
}

func (l *Loader) validateCursorTables(columns []*sql.ColumnType, schemaName string, cursorTableName string) (err error) {
	if len(columns) != 4 {
		return &SystemTableError{fmt.Errorf("table requires 4 columns ('id', 'cursor', 'block_num', 'block_id')")}
	}
	columnsCheck := map[string]string{
		"block_num": "int64",
		"block_id":  "string",
		"cursor":    "string",
		"id":        "string",
	}
	for _, f := range columns {
		columnName := f.Name()
		if _, found := columnsCheck[columnName]; !found {
			return &SystemTableError{fmt.Errorf("unexpected column %q in cursors table", columnName)}
		}
		expectedType := columnsCheck[columnName]
		actualType := f.ScanType().Kind().String()
		if expectedType != actualType {
			return &SystemTableError{fmt.Errorf("column %q has invalid type, expected %q has %q", columnName, expectedType, actualType)}
		}
		delete(columnsCheck, columnName)
	}
	if len(columnsCheck) != 0 {
		for k := range columnsCheck {
			return &SystemTableError{fmt.Errorf("missing column %q from cursors", k)}
		}
	}
	key, err := l.dialect.GetPrimaryKey(l.DB, schemaName, cursorTableName)
	if err != nil {
		return &SystemTableError{fmt.Errorf("failed getting primary key: %w", err)}
	}
	if len(key) == 0 {
		return &SystemTableError{fmt.Errorf("primary key not found: %w", err)}
	}
	if key[0] != "id" {
		return &SystemTableError{fmt.Errorf("column 'id' should be primary key not %q", key[0])}
	}
	return nil
}

func (l *Loader) GetColumnsForTable(name string) []string {
	columns := make([]string, 0, len(l.tables[name].columnsByName))
	for column := range l.tables[name].columnsByName {
		// check if column is empty
		if len(column) > 0 {
			columns = append(columns, column)
		}
	}
	return columns
}

func (l *Loader) GetAvailableTablesInSchema() []string {
	tables := make([]string, len(l.tables))
	i := 0
	for table := range l.tables {
		tables[i] = table
		i++
	}
	return tables
}

func (l *Loader) HasTable(tableName string) bool {
	if _, found := l.tables[tableName]; found {
		return true
	}
	return false
}

func (l *Loader) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddUint64("entries_count", l.entriesCount)
	return nil
}

// Setup creates the schemaName, cursors and history table where the <schemaBytes> is a byte array
// taken from somewhere.
func (l *Loader) Setup(ctx context.Context, schemaName string, userSql string, withPostgraphile bool) error {
	if userSql != "" {
		if err := l.dialect.ExecuteSetupScript(ctx, l, userSql); err != nil {
			return fmt.Errorf("exec userSql: %w", err)
		}
	}

	if err := l.setupCursorTable(ctx, schemaName, withPostgraphile); err != nil {
		return fmt.Errorf("setup cursor table: %w", err)
	}

	if err := l.setupHistoryTable(ctx, schemaName, withPostgraphile); err != nil {
		return fmt.Errorf("setup history table: %w", err)
	}

	if d, ok := l.dialect.(*ClickhouseDialect); ok && d.isStateful() {
		if _, err := l.ExecContext(ctx, d.getCreateJournalQuery()); err != nil {
			return fmt.Errorf("setup journal table: %w", err)
		}
	}

	return nil
}

func (l *Loader) setupCursorTable(ctx context.Context, schemaName string, withPostgraphile bool) error {
	query := l.dialect.GetCreateCursorQuery(schemaName, withPostgraphile)
	_, err := l.ExecContext(ctx, query)
	return err
}

func (l *Loader) setupHistoryTable(ctx context.Context, schemaName string, withPostgraphile bool) error {
	if !l.dialect.Capabilities().Reorgs {
		return nil
	}
	query := l.dialect.GetCreateHistoryQuery(schemaName, withPostgraphile)
	_, err := l.ExecContext(ctx, query)
	return err
}

// GetIdentifier returns <database>/<schema> suitable for user presentation
func (l *Loader) GetIdentifier() string {
	return fmt.Sprintf("%s/%s", l.dsn.schema, l.dsn.schema)
}

// GetIdentifier returns <database>/<schema> suitable for user presentation
func (l *Loader) GetDSN() *DSN {
	return l.dsn
}

// SetCurrentBlock sets the block the changes added next come from.
func (l *Loader) SetCurrentBlock(blockNum uint64) {
	l.currentBlockNum = blockNum
}

// NextBatchOrdinal returns the next ordinal for the current batch and increments the counter
func (l *Loader) NextBatchOrdinal() uint64 {
	ordinal := l.batchOrdinal
	l.batchOrdinal++
	return ordinal
}

type obfuscatedString string

func (s obfuscatedString) String() string {
	if len(s) == 0 {
		return "<unset>"
	}

	return "********"
}
