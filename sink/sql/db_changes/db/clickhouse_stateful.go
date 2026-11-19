package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.uber.org/zap"
)

// ClickHouse stateful mode
//
// The sink computes each row's new state itself (evaluateOperation) and stores full rows:
//
//   - keyed tables, ReplacingMergeTree(_version_, _deleted_) ordered by the primary key, take every
//     operation. A flush reads the current rows of its keys, then writes each changed row whole
//     with a higher _version_; a delete writes _deleted_ = 1.
//   - append tables, MergeTree or CollapsingMergeTree(_sign_), take inserts only.
//
// Crash safety, without transactions across tables: a flush first journals the rows it changes as
// they were and the cursor it starts from. One whose cursor is not written is rolled back by
// writing those rows again at _version_ 2*seq+1, above the flush's 2*seq, and removing its append
// rows (clickhouse_stateful_journal.go).
//
// Reorgs: a history table keeps each keyed row as it was before each reversible block; a revert
// writes them back and removes the append rows of reverted blocks (clickhouse_stateful_revert.go).
//
// The writer lock is a lease in ClickHouse Keeper (clickhouse_writer_lock.go).

const (
	statefulVersionColumn  = "_version_"
	statefulDeletedColumn  = "_deleted_"
	statefulBlockNumColumn = "_block_num_"
	statefulSignColumn     = "_sign_"

	// DefaultJournalTableName is the journal table of ClickHouse stateful mode.
	DefaultJournalTableName = "substreams_journal"

	journalPruneInterval = 1000
	journalKeptEntries   = 100
	historyPruneInterval = 100

	// statefulParallelism is how many tables a flush reads, or writes, at once.
	statefulParallelism = 8

	typeUInt64 = "UInt64"
)

type tableRole int

const (
	tableRoleAppend tableRole = iota
	tableRoleKeyed
	// tableRoleAppendSigned rows are removed by -1 cancel rows, which materialized views see.
	tableRoleAppendSigned
)

func (r tableRole) String() string {
	switch r {
	case tableRoleKeyed:
		return "keyed"
	case tableRoleAppendSigned:
		return "append (signed)"
	}
	return "append"
}

// clickhouseStateful is the state of ClickHouse stateful mode, shared by the dialect's copies.
type clickhouseStateful struct {
	journalTable string
	historyTable string
	reorgs       bool
	roles        map[string]tableRole
	// unsupported holds why a table suits no role; it is an error only once the module writes it.
	unsupported map[string]error
	// defaults holds, per table, the constant DEFAULT of each column that has one.
	defaults       map[string]map[string]string
	blockNumColumn map[string]bool

	nextSeq uint64
	// uncommitted is the last journal entry until its cursor is written.
	uncommitted *journalEntry
}

// EnableClickhouseStateful turns on ClickHouse stateful mode, before LoadTables. With handleReorgs,
// reversible blocks are written and reverted on undo through historyTableName.
func (l *Loader) EnableClickhouseStateful(journalTableName, historyTableName string, handleReorgs bool) error {
	d, ok := l.dialect.(*ClickhouseDialect)
	if !ok {
		return errors.New("stateful mode is only for ClickHouse")
	}
	d.stateful = &clickhouseStateful{
		journalTable:   journalTableName,
		historyTable:   historyTableName,
		reorgs:         handleReorgs,
		roles:          map[string]tableRole{},
		unsupported:    map[string]error{},
		defaults:       map[string]map[string]string{},
		blockNumColumn: map[string]bool{},
	}
	l.handleReorgs = handleReorgs
	return nil
}

// PrepareClickhouseStateful, after LoadTables, gives the tables their roles in stateful mode.
func (l *Loader) PrepareClickhouseStateful(ctx context.Context) error {
	d, ok := l.dialect.(*ClickhouseDialect)
	if !ok || d.stateful == nil {
		return nil
	}
	return d.classifyTables(ctx, l)
}

// synchronousInserts makes inserts return once written: the journal, rows and cursor are written in
// that order.
func synchronousInserts(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"async_insert": 0}))
}

func (d ClickhouseDialect) isStateful() bool {
	return d.stateful != nil
}

func (d ClickhouseDialect) journalTableIdentifier() string {
	return EscapeIdentifier(d.schemaName) + "." + EscapeIdentifier(d.stateful.journalTable)
}

func (d ClickhouseDialect) historyTableIdentifier() string {
	return EscapeIdentifier(d.schemaName) + "." + EscapeIdentifier(d.stateful.historyTable)
}

// getCreateHistoryQuery creates the history table: for each keyed row changed in a reversible
// block, the row as it was before that block (prev_value, a JSON Row, NULL when it did not exist).
// seq is the flush that wrote it, so a rollback can remove it; a replay writes the same rows again.
func (d ClickhouseDialect) getCreateHistoryQuery() string {
	clusterClause := ""
	engine := "ReplacingMergeTree(seq)"
	if d.cluster != "" {
		clusterClause = "ON CLUSTER " + EscapeIdentifier(d.cluster)
		engine = "ReplicatedReplacingMergeTree(seq)"
	}

	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s %s
		(
			table_name  String,
			pk          String,
			block_num   UInt64,
			prev_value  Nullable(String),
			seq         UInt64
		) ENGINE = %s ORDER BY (table_name, pk, block_num)
	`, d.historyTableIdentifier(), clusterClause, engine)
}

func (d ClickhouseDialect) getCreateJournalQuery() string {
	clusterClause := ""
	engine := "MergeTree()"
	if d.cluster != "" {
		clusterClause = "ON CLUSTER " + EscapeIdentifier(d.cluster)
		engine = "ReplicatedMergeTree()"
	}

	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s %s
		(
			module_hash   String,
			seq           UInt64,
			base_cursor   String,
			payload       String CODEC(ZSTD(3)),
			created_at    DateTime DEFAULT now()
		) ENGINE = %s ORDER BY (module_hash, seq)
	`, d.journalTableIdentifier(), clusterClause, engine)
}

// classifyTables gives each table the module can write its role.
func (d ClickhouseDialect) classifyTables(ctx context.Context, l *Loader) error {
	rows, err := l.QueryContext(ctx, `
		SELECT name, engine, engine_full, sorting_key, dependencies_table
		FROM system.tables
		WHERE database = ?`, d.schemaName)
	if err != nil {
		return fmt.Errorf("reading table engines: %w", err)
	}
	defer rows.Close()

	type engineInfo struct {
		engine, engineFull, sortingKey string
		views                          []string
	}
	engines := map[string]engineInfo{}
	for rows.Next() {
		var name string
		var info engineInfo
		if err := rows.Scan(&name, &info.engine, &info.engineFull, &info.sortingKey, &info.views); err != nil {
			return fmt.Errorf("reading table engines: %w", err)
		}
		engines[name] = info
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading table engines: %w", err)
	}

	for name, table := range l.tables {
		if name == d.cursorTableName || name == d.stateful.journalTable || name == d.stateful.historyTable {
			continue
		}
		info, found := engines[name]
		if !found {
			return fmt.Errorf("table %q: not found in system.tables", name)
		}

		managed, err := l.statefulColumns(ctx, name)
		if err != nil {
			return fmt.Errorf("reading the sink's columns of %q: %w", name, err)
		}
		role, err := tableRoleFor(table, info.engine, info.engineFull, info.sortingKey, managed, d.stateful.reorgs)
		if err != nil {
			d.stateful.unsupported[name] = fmt.Errorf("table %q cannot be written in ClickHouse stateful mode: %w", name, err)
			l.logger.Info("table not writable in clickhouse stateful mode", zap.String("table", name), zap.Error(err))
			continue
		}
		if role == tableRoleKeyed {
			if err := checkKeyedColumns(table); err != nil {
				d.stateful.unsupported[name] = fmt.Errorf("table %q cannot be written in ClickHouse stateful mode: %w", name, err)
				l.logger.Info("table not writable in clickhouse stateful mode", zap.String("table", name), zap.Error(err))
				continue
			}
		}
		d.stateful.roles[name] = role
		d.stateful.blockNumColumn[name] = managed[statefulBlockNumColumn] == typeUInt64
		if d.stateful.defaults[name], err = l.constantColumnDefaults(ctx, name); err != nil {
			return fmt.Errorf("reading column defaults of %q: %w", name, err)
		}
		l.logger.Info("clickhouse stateful table", zap.String("table", name), zap.Stringer("role", role))

		if len(info.views) > 0 {
			l.logger.Warn("materialized views read this table: they see every row the sink writes, including each new version of a keyed row, and rows removed from an append table without a _sign_ column stay in them, see the ClickHouse Stateful Mode docs",
				zap.String("table", name), zap.Strings("views", info.views))
		}
	}
	return nil
}

// tableRoleFor works out a table's role from its engine and its sink-managed columns.
func tableRoleFor(table *TableInfo, engine, engineFull, sortingKey string, managedColumns map[string]string, reorgs bool) (tableRole, error) {
	if !strings.HasSuffix(engine, "MergeTree") {
		return 0, fmt.Errorf("engine %s is not a MergeTree", engine)
	}
	if managedColumns[statefulVersionColumn] != typeUInt64 {
		return 0, fmt.Errorf("add a %s UInt64 column (used by the sink)", statefulVersionColumn)
	}
	needBlockNum := func() error {
		if reorgs && managedColumns[statefulBlockNumColumn] != typeUInt64 {
			return fmt.Errorf("add a %s UInt64 column (used by the sink to revert reorged blocks)", statefulBlockNumColumn)
		}
		return nil
	}

	switch {
	case engine == "MergeTree" || engine == "ReplicatedMergeTree" || engine == "SharedMergeTree":
		return tableRoleAppend, needBlockNum()

	case strings.HasSuffix(engine, "CollapsingMergeTree") && !strings.HasSuffix(engine, "VersionedCollapsingMergeTree"):
		if engineArguments(engineFull, "CollapsingMergeTree(") == nil || lastOf(engineArguments(engineFull, "CollapsingMergeTree(")) != statefulSignColumn {
			return 0, fmt.Errorf("use ENGINE = CollapsingMergeTree(%s) for an append table with cancel rows", statefulSignColumn)
		}
		if managedColumns[statefulSignColumn] != "Int8" {
			return 0, fmt.Errorf("add a %s Int8 column (used by the sink)", statefulSignColumn)
		}
		return tableRoleAppendSigned, needBlockNum()

	case !strings.HasSuffix(engine, "ReplacingMergeTree"):
		// Other MergeTree variants merge rows together, which would mix the versions a rollback
		// removes by.
		return 0, fmt.Errorf("engine %s merges rows: use MergeTree (or CollapsingMergeTree(%s)) for a table taking inserts only, or ReplacingMergeTree(%s, %s) for one taking every operation", engine, statefulSignColumn, statefulVersionColumn, statefulDeletedColumn)
	}

	if !replacingEngineUsesVersionAndDeleted(engineFull) {
		return 0, fmt.Errorf("use ENGINE = ReplacingMergeTree(%s, %s) so the sink's versions and deletes apply", statefulVersionColumn, statefulDeletedColumn)
	}
	if managedColumns[statefulDeletedColumn] != "UInt8" {
		return 0, fmt.Errorf("add a %s UInt8 column (used by the sink)", statefulDeletedColumn)
	}

	sortingColumns := splitSortingKey(sortingKey)
	keyColumns := make([]string, len(table.primaryColumns))
	for i, c := range table.primaryColumns {
		keyColumns[i] = c.name
	}
	slices.Sort(sortingColumns)
	slices.Sort(keyColumns)
	if !slices.Equal(sortingColumns, keyColumns) {
		return 0, fmt.Errorf("the sorting key (ORDER BY %s) must be the primary key, as rows are replaced by sorting key", sortingKey)
	}
	return tableRoleKeyed, nil
}

// checkKeyedColumns refuses keyed tables with a column the sink cannot read back and write again
// (Map, IPv4), as every write of a row rewrites all of it.
func checkKeyedColumns(table *TableInfo) error {
	for _, c := range sortedColumns(table) {
		stored, err := storedZeroValue(c)
		if err == nil {
			_, err = statefulColumnValue(c, stored)
		}
		if err != nil {
			return fmt.Errorf("column %q of type %s cannot be read back and written again by the sink: %w", c.name, c.databaseTypeName, err)
		}
	}
	return nil
}

func replacingEngineUsesVersionAndDeleted(engineFull string) bool {
	parts := engineArguments(engineFull, "ReplacingMergeTree(")
	// Replicated engines take the ZooKeeper path and replica name first.
	return len(parts) >= 2 && parts[len(parts)-2] == statefulVersionColumn && parts[len(parts)-1] == statefulDeletedColumn
}

// engineArguments gives the arguments of the engine in engineFull opening with prefix, nil if none.
func engineArguments(engineFull, prefix string) []string {
	_, rest, found := strings.Cut(engineFull, prefix)
	if !found {
		return nil
	}
	args, _, found := strings.Cut(rest, ")")
	if !found {
		return nil
	}
	var parts []string
	for p := range strings.SplitSeq(args, ",") {
		parts = append(parts, unquoteIdentifier(p))
	}
	return parts
}

// unquoteIdentifier trims an identifier from a ClickHouse expression of its spaces and quotes.
func unquoteIdentifier(s string) string {
	return strings.Trim(strings.TrimSpace(s), "`\"")
}

func lastOf(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func splitSortingKey(sortingKey string) []string {
	var out []string
	for p := range strings.SplitSeq(sortingKey, ",") {
		if p = unquoteIdentifier(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// statefulColumns gives the types of a table's sink-managed columns, which GetTableColumns leaves
// out of the table's columns.
func (l *Loader) statefulColumns(ctx context.Context, table string) (map[string]string, error) {
	rows, err := l.QueryContext(ctx, `SELECT name, type FROM system.columns WHERE database = ? AND table = ? AND name IN (?, ?, ?, ?)`,
		l.dsn.Schema(), table, statefulVersionColumn, statefulDeletedColumn, statefulBlockNumColumn, statefulSignColumn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return nil, err
		}
		out[name] = typ
	}
	return out, rows.Err()
}

// constantColumnDefaults gives the columns of a table whose DEFAULT is a constant (a number, a
// string, a boolean), with their value. Other defaults (now(), say) are not applied.
func (l *Loader) constantColumnDefaults(ctx context.Context, table string) (map[string]string, error) {
	rows, err := l.QueryContext(ctx, `SELECT name, default_expression FROM system.columns WHERE database = ? AND table = ? AND default_kind = 'DEFAULT'`,
		l.dsn.Schema(), table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, expression string
		if err := rows.Scan(&name, &expression); err != nil {
			return nil, err
		}
		if value, ok := constantDefault(expression); ok {
			out[name] = value
		}
	}
	return out, rows.Err()
}

var numberLiteralRegex = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

func constantDefault(expression string) (string, bool) {
	e := strings.TrimSpace(expression)
	switch {
	case numberLiteralRegex.MatchString(e):
		return e, true
	case e == "true" || e == "false":
		return e, true
	case len(e) >= 2 && e[0] == '\'' && e[len(e)-1] == '\'' && !strings.Contains(e[1:len(e)-1], "'"):
		return e[1 : len(e)-1], true
	}
	return "", false
}

func isStatefulManagedColumn(name string) bool {
	switch name {
	case statefulVersionColumn, statefulDeletedColumn, statefulBlockNumColumn, statefulSignColumn:
		return true
	}
	return false
}
