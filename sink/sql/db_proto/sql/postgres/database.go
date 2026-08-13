package postgres

import (
	"context"
	pgsql "database/sql"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lib/pq"
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
	// pool is only created when the spool is enabled: binary COPY needs pgx, the rest of
	// the sink talks to PostgreSQL through database/sql and lib/pq.
	pool         *pgxpool.Pool
	spoolOptions *spool.Options
	tx           *pgsql.Tx
	dsn          *db.DSN
	schema       *schema.Schema
	logger       *zap.Logger
	dialect      *DialectPostgres
	inserter     pgInserter
	flusher      pgFlusher
	constraints  sql.ConstraintPolicy

	// writeMode is what the operator asked for; resolvedWriteMode is what Open settled on
	// and is what the startup log reports.
	writeMode         sql.WriteMode
	resolvedWriteMode sql.WriteMode
}

// bufferActive reports whether rows are currently being routed to the spool.
//
// This is deliberately not "was a spool configured": schema creation runs before Open
// and does need real transactions, so the distinction is what keeps DDL working.
func (d *Database) bufferActive() bool {
	_, ok := d.inserter.(*localBufferInserter)

	return ok
}

// WithSpool turns on the on-disk spool. It must be called before Open.
func (d *Database) WithSpool(options spool.Options) {
	d.spoolOptions = &options
}

// WithWriteMode records how sealed segments should reach the database. It must be called
// before Open, which is where the mode is resolved and validated against the schema.
func (d *Database) WithWriteMode(mode sql.WriteMode) error {
	parsed, err := sql.ParseWriteMode(string(mode))
	if err != nil {
		return err
	}
	d.writeMode = parsed

	return nil
}

// WriteMode reports the mode Open settled on, for the startup log.
func (d *Database) WriteMode() sql.WriteMode { return d.resolvedWriteMode }

// tablesOrderable reports whether the schema's tables can be put in an order that keeps a
// referenced table ahead of the tables referencing it.
//
// Grouping rows by table — which both the spool and the multi-row INSERT path do — only
// works under foreign keys if such an order exists. A cycle has none, and only the
// row-at-a-time inserter can cope, because it follows the walk, which produces a parent
// before its children.
func (d *Database) tablesOrderable() (bool, error) {
	if d.constraints.DisableForeignKeys {
		return true, nil
	}

	if _, err := d.dialect.TableApplyOrder(); err != nil {
		return false, err
	}

	return true, nil
}

// spoolFormat is the on-disk layout each write mode needs.
//
// The spool always holds bytes that are ready to send, so the format follows the mode
// rather than the other way round: pre-rendering is worth ~8% for COPY and the same
// argument carries to the INSERT modes, where the rendering the accumulator used to do at
// flush time simply happens earlier.
func spoolFormat(mode sql.WriteMode) spool.Format {
	switch mode {
	case sql.WriteModeBatchInsert:
		return spool.FormatTuples
	case sql.WriteModeRowInsert:
		return spool.FormatRowLog
	default:
		return spool.FormatPGCopy
	}
}

// resolveWriteMode turns the requested mode into the one Open will use.
//
// An explicit mode the schema cannot support is an error rather than a downgrade: a
// silent fall back to row-at-a-time inserts is an order of magnitude slower, and finding
// that out from a log line three days into a backfill is not finding it out.
func (d *Database) resolveWriteMode() (sql.WriteMode, error) {
	orderable, orderErr := d.tablesOrderable()

	switch d.writeMode {
	case sql.WriteModeRowInsert:
		return sql.WriteModeRowInsert, nil

	case sql.WriteModeCopy, sql.WriteModeBatchInsert:
		if !orderable {
			return "", fmt.Errorf("--write-mode=%s groups rows by table, which needs the schema's tables to be orderable by their foreign keys, and this schema's cannot be (%w). "+
				"Use --disable-foreign-keys, or --write-mode=%s, which replays the walk instead and so always keeps a parent ahead of its children",
				d.writeMode, orderErr, sql.WriteModeRowInsert)
		}

		return d.writeMode, nil

	default:
		if !orderable {
			d.logger.Info("the schema's foreign keys cannot be ordered, replaying the walk one row at a time",
				zap.String("write_mode", string(sql.WriteModeRowInsert)), zap.Error(orderErr))

			return sql.WriteModeRowInsert, nil
		}
		if d.spoolOptions == nil {
			return sql.WriteModeBatchInsert, nil
		}

		return sql.WriteModeCopy, nil
	}
}

func NewDatabase(schema *schema.Schema, dsn *db.DSN, moduleOutputType string, rootMessageDescriptor protoreflect.MessageDescriptor, useProtoOptions bool, constraints sql.ConstraintPolicy, bytesEncoding bytes.Encoding, logger *zap.Logger) (*Database, error) {
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
		db:           sqlDB,
		dsn:          dsn,
		schema:       schema,
		constraints:  constraints,
		BaseDatabase: baseDB,
		dialect:      dialect,
		logger:       logger,
	}

	return database, nil
}

func (d *Database) Open() error {
	mode, err := d.resolveWriteMode()
	if err != nil {
		return err
	}
	d.resolvedWriteMode = mode

	if d.spoolOptions == nil {
		return d.openDirectInserter()
	}
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

	inserter, err := newLocalBufferInserter(ctx, d, spoolFormat(mode), *d.spoolOptions, d.logger)
	if err != nil {
		return fmt.Errorf("starting the local spool: %w", err)
	}
	d.inserter = inserter
	d.flusher = inserter

	return nil
}

// openDirectInserter installs the inserter that writes to the database itself: one
// prepared INSERT per row in row-insert mode, a multi-row INSERT built at flush otherwise.
//
// Constraints used to mean one prepared INSERT per row, which the benchmarks put at a
// tenth of the multi-row path. That was only ever needed for ordering: the walk produces a
// parent before its children, and inserting row by row preserved it. Ordering the
// multi-row statements by the foreign keys themselves preserves it too, so row-at-a-time
// is now reserved for a schema whose references cannot be ordered at all.
func (d *Database) openDirectInserter() error {
	// The chain-head switch reopens this after the spool is gone, and copy is not a
	// direct mode, so it lands on the multi-row path.
	if d.resolvedWriteMode == sql.WriteModeRowInsert {
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

// SwitchToDirectInserts drains the spool and inserts straight into the database from here
// on.
//
// The spool trades freshness for throughput: rows sit on disk until a segment fills,
// which is what a backfill wants and the opposite of what a sink at the chain head wants,
// where a block should be queryable when it arrives rather than when the segment it
// happens to land in is full. Reorgs are also cheaper to undo out of a table than out of
// a segment that has not been applied yet.
//
// Everything spooled is applied before the switch, so no row is left behind, and the
// spool is closed for good — a stream that has reached the head does not go back.
func (d *Database) SwitchToDirectInserts(ctx context.Context) error {
	inserter, ok := d.inserter.(*localBufferInserter)
	if !ok {
		return nil
	}

	d.logger.Info("stream reached the chain head, draining the spool and switching to direct inserts. "+
		"--write-mode, --db-write-* and --spool-* no longer apply from here on",
		zap.String("write_mode_was", string(d.resolvedWriteMode)))

	if err := inserter.close(ctx); err != nil {
		return fmt.Errorf("draining the spool: %w", err)
	}

	// The spool is what copy mode is; without it the direct path is the multi-row one.
	if d.resolvedWriteMode == sql.WriteModeCopy {
		d.resolvedWriteMode = sql.WriteModeBatchInsert
	}

	if err := d.openDirectInserter(); err != nil {
		return fmt.Errorf("switching to direct inserts: %w", err)
	}

	// bufferActive() reads the inserter, so transactions resume from here; the options
	// go with it to keep the two from disagreeing.
	d.spoolOptions = nil

	if d.pool != nil {
		d.pool.Close()
		d.pool = nil
	}

	return nil
}

func (d *Database) GetDialect() sql.Dialect {
	return d.dialect
}

func (d *Database) CreateDatabase(applyConstraints bool) error {
	err := d.createDatabase()
	if err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	if applyConstraints {
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
// It is what `sink postgres apply-constraints` does to a database synced without them: the sink
// info hash cannot tell those two apart, since it is computed over the DDL the dialect
// would emit, constraints included, either way.
//
// The caller owns the transaction. On a populated database this is not quick — every
// index has to be built and every foreign key validated, with the table locked meanwhile.
func (d *Database) ApplyConstraints() error {
	return d.applyConstraints()
}

// applyConstraints creates what the policy asks for and the catalog does not have.
//
// It owns its transactions rather than running inside the caller's, and commits every
// --constraints-per-transaction statements. Building an index and validating a foreign key
// are the two most memory-hungry things the sink ever asks of the server, and holding them
// all open at once is what turns a large schema into an OOM that loses the entire pass.
// Committing as it goes bounds that, and leaves a killed run's finished work in place for
// the next one to carry on from.
func (d *Database) applyConstraints() error {
	startAt := time.Now()

	existing, err := d.existingConstraints(d.querier())
	if err != nil {
		return err
	}

	var pending []statement
	collect := func(kind string, constraints []*sql.Constraint, skip func(string) bool) {
		for _, constraint := range constraints {
			if skip(constraint.Table) {
				d.logger.Debug("constraint disabled by the policy, skipping", zap.String("kind", kind), zap.String("table", constraint.Table))
				continue
			}

			if key, ok := constraintTarget(constraint.Sql); ok && existing[key] {
				d.logger.Debug("constraint already in place, skipping", zap.String("constraint", key.name), zap.String("relation", key.relation))
				continue
			}

			pending = append(pending, statement{kind: kind, sql: constraint.Sql})
		}
	}

	collect("pk", d.dialect.PrimaryKeySql, d.constraints.SkipPrimaryKey)
	collect("unique", d.dialect.UniqueConstraintSql, d.constraints.SkipUnique)
	collect("fk constraint", d.dialect.ForeignKeySql, d.constraints.SkipForeignKey)

	if err := d.runStatements(pending); err != nil {
		return err
	}

	d.logger.Info("applying constraints", zap.Int("created", len(pending)), zapx.HumanDuration("duration", time.Since(startAt)))

	return nil
}

// EnsureBlockNumberIndexes creates the index the sink needs for its own reorg path, on
// every table, when the sink starts.
//
// It is not part of the constraint pass and not governed by --apply-constraints. Those
// describe the schema and are the operator's to schedule; this one the sink depends on to
// undo a reorg without sequentially scanning every table, so waiting for a maintenance
// window would mean running without it for as long as the operator likes.
//
// Concurrently, and therefore outside any transaction: a restart onto an already-loaded
// table must not lock out the writers. On a schema that already has them this is one
// catalog query and nothing else.
func (d *Database) EnsureBlockNumberIndexes(ctx context.Context) error {
	if d.constraints.DisableBlockNumberIndex {
		return nil
	}

	existing, err := d.existingIndexes(d.db)
	if err != nil {
		return err
	}

	invalid, err := d.invalidIndexes()
	if err != nil {
		return err
	}

	startAt := time.Now()
	created := 0

	for _, index := range d.dialect.IndexSql {
		key, ok := indexTarget(index.Sql)
		if !ok {
			continue
		}

		if invalid[key.name] {
			// A concurrent build that was interrupted leaves an index behind that no query
			// will ever use, and IF NOT EXISTS would happily keep it forever.
			d.logger.Info("dropping an index a previous concurrent build left unusable", zap.String("index", key.name))
			if _, err := d.db.ExecContext(ctx, fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s.%s",
				pq.QuoteIdentifier(d.schema.Name), pq.QuoteIdentifier(key.name))); err != nil {
				return fmt.Errorf("dropping the invalid index %q: %w", key.name, err)
			}
		} else if existing[key] {
			continue
		}

		d.logger.Info("creating the block number index", zap.String("index", key.name), zap.String("sql", index.Sql))
		if _, err := d.db.ExecContext(ctx, index.Sql); err != nil {
			return fmt.Errorf("creating the index %q: %w", key.name, err)
		}
		created++
	}

	if created > 0 {
		d.logger.Info("block number indexes created", zap.Int("created", created), zapx.HumanDuration("duration", time.Since(startAt)))
	}

	return nil
}

// invalidIndexes names the indexes an interrupted concurrent build left behind. They exist
// as far as IF NOT EXISTS is concerned and are used by nothing.
func (d *Database) invalidIndexes() (map[string]bool, error) {
	rows, err := d.db.Query(`
		SELECT cl.relname
		FROM pg_index i
		JOIN pg_class cl ON cl.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = cl.relnamespace
		WHERE n.nspname = $1 AND NOT i.indisvalid`, d.schema.Name)
	if err != nil {
		return nil, fmt.Errorf("listing the invalid indexes of schema %q: %w", d.schema.Name, err)
	}
	defer rows.Close()

	invalid := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning an invalid index name: %w", err)
		}
		invalid[name] = true
	}

	return invalid, rows.Err()
}

// existingIndexes reads which indexes the schema carries. They live in pg_indexes rather
// than pg_constraint, a plain index not being a constraint.
func (d *Database) existingIndexes(from rowQuerier) (map[constraintKey]bool, error) {
	rows, err := from.Query(`
		SELECT indexname, schemaname || '.' || tablename
		FROM pg_indexes
		WHERE schemaname = $1`, d.schema.Name)
	if err != nil {
		return nil, fmt.Errorf("listing the existing indexes of schema %q: %w", d.schema.Name, err)
	}
	defer rows.Close()

	existing := map[constraintKey]bool{}
	for rows.Next() {
		var name, relation string
		if err := rows.Scan(&name, &relation); err != nil {
			return nil, fmt.Errorf("scanning index name: %w", err)
		}
		existing[constraintKey{relation: normalizeRelation(relation), name: name}] = true
	}

	return existing, rows.Err()
}

// indexTargetPattern pulls the name and the relation out of the dialect's own CREATE INDEX.
var indexTargetPattern = regexp.MustCompile(`(?i)create\s+index\s+(?:concurrently\s+)?(?:if\s+not\s+exists\s+)?"?([a-z0-9_]+)"?\s+on\s+(\S+)`)

func indexTarget(statement string) (constraintKey, bool) {
	match := indexTargetPattern.FindStringSubmatch(statement)
	if len(match) != 3 {
		return constraintKey{}, false
	}

	return constraintKey{relation: normalizeRelation(match[2]), name: match[1]}, true
}

// statement is one piece of DDL the constraint pass has to run.
type statement struct {
	kind string
	sql  string
}

// runStatements executes the DDL, committing every ConstraintsPerTransaction of them.
//
// When the caller already has a transaction open — creating the schema does — the
// statements join it instead: the tables they constrain are not committed yet, so
// nothing else could see them.
func (d *Database) runStatements(statements []statement) error {
	if d.tx != nil {
		for _, s := range statements {
			d.logger.Info("executing "+s.kind+" statement", zap.String("sql", s.sql))
			if _, err := d.tx.Exec(s.sql); err != nil {
				return fmt.Errorf("executing %s statement: %w %s", s.kind, err, s.sql)
			}
		}

		return nil
	}

	perTransaction := d.constraints.ConstraintsPerTransaction()

	for start := 0; start < len(statements); start += perTransaction {
		end := min(start+perTransaction, len(statements))

		tx, err := d.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning a constraint transaction: %w", err)
		}

		for _, s := range statements[start:end] {
			startAt := time.Now()
			d.logger.Info("executing "+s.kind+" statement", zap.String("sql", s.sql))

			if _, err := tx.Exec(s.sql); err != nil {
				tx.Rollback()
				return fmt.Errorf("executing %s statement: %w %s", s.kind, err, s.sql)
			}

			d.logger.Debug("statement done", zap.String("sql", s.sql), zapx.HumanDuration("duration", time.Since(startAt)))
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing a constraint transaction: %w", err)
		}
	}

	return nil
}

// MissingConstraints names the constraints the policy says this schema should carry and
// the catalog does not have.
//
// It is one query against pg_constraint filtered by namespace — indexed, and nothing like
// the cost of building the constraints it reports on — so it is cheap enough to run on
// every start.
func (d *Database) MissingConstraints() ([]string, error) {
	existing, err := d.existingConstraints(d.querier())
	if err != nil {
		return nil, err
	}

	var missing []string
	collect := func(constraints []*sql.Constraint, skip func(string) bool) {
		for _, constraint := range constraints {
			if skip(constraint.Table) {
				continue
			}
			if key, ok := constraintTarget(constraint.Sql); ok && !existing[key] {
				missing = append(missing, key.relation+"."+key.name)
			}
		}
	}

	collect(d.dialect.PrimaryKeySql, d.constraints.SkipPrimaryKey)
	collect(d.dialect.UniqueConstraintSql, d.constraints.SkipUnique)
	collect(d.dialect.ForeignKeySql, d.constraints.SkipForeignKey)

	return missing, nil
}

// DropConstraints removes the constraints this schema's DDL would create, leaving
// anything the sink did not put there alone.
//
// Foreign keys go first, then unique constraints, then primary keys: a primary key still
// referenced by a foreign key cannot be dropped. Constraints already absent are skipped,
// so this is idempotent and safe to run against a schema that never had them.
func (d *Database) DropConstraints() error {
	existing, err := d.existingConstraints(d.querier())
	if err != nil {
		return err
	}

	var pending []statement
	collect := func(kind string, constraints []*sql.Constraint) {
		for _, constraint := range constraints {
			key, ok := constraintTarget(constraint.Sql)
			if !ok || !existing[key] {
				d.logger.Debug("constraint already absent, skipping", zap.String("constraint", key.name))
				continue
			}

			pending = append(pending, statement{
				kind: kind,
				sql:  fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", key.relation, pq.QuoteIdentifier(key.name)),
			})
		}
	}

	// Foreign keys first, then uniques, then primary keys: a primary key still referenced
	// by a foreign key cannot be dropped.
	collect("fk constraint", d.dialect.ForeignKeySql)
	collect("unique constraint", d.dialect.UniqueConstraintSql)
	collect("pk", d.dialect.PrimaryKeySql)

	return d.runStatements(pending)
}

// existingConstraintRelations maps every constraint name in the schema to the relation it
// belongs to, already quoted. Reading the relation from the catalog rather than deriving
// it from the logical table name is what keeps this working under the server's own
// identifier folding.
// rowQuerier is what both a transaction and the pool itself satisfy.
type rowQuerier interface {
	Query(query string, args ...any) (*pgsql.Rows, error)
}

// querier is the transaction if one is open, the pool otherwise.
//
// Creating the schema runs the constraint pass inside its own transaction, against tables
// that are not committed yet — so the pass has to read and write through that transaction
// or it cannot see them. On a database that is already loaded there is no open
// transaction, and the pass owns its own.
func (d *Database) querier() rowQuerier {
	if d.tx != nil {
		return d.tx
	}

	return d.db
}

// constraintKey identifies a constraint the way PostgreSQL does: by relation and name.
//
// Names are only unique per table, not per schema — every table carries a foreign key
// called fk_block — so keying by name alone both drops the wrong number of them and
// reports a constraint present on one table as present on all of them.
type constraintKey struct {
	relation string
	name     string
}

// existingConstraints reads which of them the schema actually carries.
func (d *Database) existingConstraints(from rowQuerier) (map[constraintKey]bool, error) {
	rows, err := from.Query(`
		SELECT c.conname, n.nspname || '.' || cl.relname
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		JOIN pg_class cl ON cl.oid = c.conrelid
		WHERE n.nspname = $1`, d.schema.Name)
	if err != nil {
		return nil, fmt.Errorf("listing the existing constraints of schema %q: %w", d.schema.Name, err)
	}
	defer rows.Close()

	existing := map[constraintKey]bool{}
	for rows.Next() {
		var name, relation string
		if err := rows.Scan(&name, &relation); err != nil {
			return nil, fmt.Errorf("scanning constraint name: %w", err)
		}
		existing[constraintKey{relation: normalizeRelation(relation), name: name}] = true
	}

	return existing, rows.Err()
}

// constraintTargetPattern pulls the relation and the name out of the dialect's own DDL.
var constraintTargetPattern = regexp.MustCompile(`(?i)alter\s+table\s+(\S+)\s+add\s+constraint\s+"?([a-z0-9_]+)"?`)

// constraintTarget says which constraint a statement of ours creates. It reports false for
// anything it does not recognise, which is then left to the server to accept or reject.
func constraintTarget(statement string) (constraintKey, bool) {
	match := constraintTargetPattern.FindStringSubmatch(statement)
	if len(match) != 3 {
		return constraintKey{}, false
	}

	return constraintKey{relation: normalizeRelation(match[1]), name: match[2]}, true
}

// normalizeRelation puts a schema-qualified name in the shape the catalog reports, the
// dialect writing its DDL unquoted and the server folding it.
func normalizeRelation(relation string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(relation), `"`, ""))
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

func (d *Database) WalkMessageDescriptorAndInsert(dm protoreflect.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent) (time.Duration, error) {
	return d.WalkMessageDescriptorAndInsertWithDialect(dm, blockNum, blockTimestamp, parent, d.dialect, d)
}

func (d *Database) WalkMessageDescriptorAndInsertInto(dm protoreflect.Message, blockNum uint64, blockTimestamp time.Time, parent *sql.Parent, inserter sql.Inserter) (time.Duration, error) {
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
// that foreign key only exists once the constraints have been created. Deleting just the block rows, as
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

	// Reverse apply order: a referencing table is emptied before the table it points at,
	// so a foreign key never blocks its own cleanup. Nesting depth would order the
	// parent-child keys and quietly get sibling references wrong.
	ordered, err := d.dialect.TableApplyOrder()
	if err != nil {
		return err
	}

	var rowsAffected int64
	for i := len(ordered) - 1; i >= 0; i-- {
		name := ordered[i]
		if name == sql.DialectTableBlock {
			// Deleted last, below, and counted separately.
			continue
		}

		query := fmt.Sprintf(`DELETE FROM %s WHERE %s > $1`, tableName(d.schema.Name, name), sql.DialectFieldBlockNumber)
		result, err := tx.Exec(query, lastValidBlockNum)
		if err != nil {
			return fmt.Errorf("deleting rows of %q from %d: %w", name, lastValidBlockNum, err)
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
