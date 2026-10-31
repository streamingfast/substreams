package db

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"golang.org/x/sync/errgroup"
)

// flushStateful is Flush in stateful mode.
func (d ClickhouseDialect) flushStateful(ctx context.Context, l *Loader, outputModuleHash string, lastFinalBlock uint64) (int, error) {
	ctx = synchronousInserts(ctx)
	st := d.stateful
	if err := l.checkWriterLease(ctx, d); err != nil {
		return 0, err
	}
	if done, err := d.resolveUncommitted(ctx, l); err != nil || done {
		return 0, err
	}
	if err := checkStoredCursor(ctx, l); err != nil {
		return 0, err
	}

	entry := d.newJournalEntry(l, outputModuleHash)
	version := 2 * entry.seq

	type keyedTableOps struct {
		table *TableInfo
		ops   *OrderedMap[string, *Operation]
	}
	var keyedTables []keyedTableOps
	var keyedWrites []rowsWrite
	var appendWrites []rowsWrite
	var history []historyRow
	count := 0

	for tablePair := l.entries.Oldest(); tablePair != nil; tablePair = tablePair.Next() {
		tableName, ops := tablePair.Key, tablePair.Value
		if ops.Len() == 0 {
			continue
		}
		table := l.tables[tableName]
		role, found := st.roles[tableName]
		if !found {
			if err := st.unsupported[tableName]; err != nil {
				return 0, err
			}
			return 0, fmt.Errorf("table %q has no stateful role", tableName)
		}

		switch role {
		case tableRoleAppend, tableRoleAppendSigned:
			write := rowsWrite{table: table}
			for opPair := ops.Oldest(); opPair != nil; opPair = opPair.Next() {
				op := opPair.Value
				if op.opType != OperationTypeInsert {
					return 0, fmt.Errorf("table %q is an append table (not a ReplacingMergeTree(%s, %s)): it only accepts inserts, got %s", tableName, statefulVersionColumn, statefulDeletedColumn, op)
				}
				row, err := newRow(op)
				if err != nil {
					return 0, err
				}
				write.rows = append(write.rows, row)
				write.blockNums = append(write.blockNums, op.blockNum)
			}
			appendWrites = append(appendWrites, write)
			entry.appendTables = append(entry.appendTables, tableName)
			count += len(write.rows)

		case tableRoleKeyed:
			keyedTables = append(keyedTables, keyedTableOps{table: table, ops: ops})
		}
	}

	results := make([]keyedTableResult, len(keyedTables))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(statefulParallelism)
	for i, k := range keyedTables {
		group.Go(func() error {
			result, err := d.evaluateKeyedTable(groupCtx, l, k.table, k.ops)
			results[i] = result
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return 0, err
	}
	for i, result := range results {
		if len(result.write.rows) > 0 {
			keyedWrites = append(keyedWrites, result.write)
			entry.beforeImages[keyedTables[i].table.name] = result.beforeImages
		}
		history = append(history, result.history...)
		count += len(result.write.rows)
	}

	if err := d.writeJournal(ctx, l, entry); err != nil {
		return 0, fmt.Errorf("writing journal: %w", err)
	}
	d.pruneJournalAndHistory(ctx, l, entry.seq, lastFinalBlock)

	// A failed write leaves the flush to be rolled back before the next attempt.
	writes, writesCtx := errgroup.WithContext(ctx)
	writes.SetLimit(statefulParallelism)
	writes.Go(func() error { return d.insertHistory(writesCtx, l, history, entry.seq) })
	for _, w := range append(keyedWrites, appendWrites...) {
		writes.Go(func() error { return d.insertRows(writesCtx, l, w, version) })
	}
	if err := writes.Wait(); err != nil {
		return 0, err
	}
	if statefulFlushTestHook != nil {
		if err := statefulFlushTestHook(); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// statefulFlushTestHook, in tests, runs once a stateful flush wrote its rows, before the cursor.
var statefulFlushTestHook func() error

// rowsWrite is a batch of full rows for a table. For a keyed table, deleted flags each row; for an
// append table, blockNums has each row's block.
type rowsWrite struct {
	table     *TableInfo
	rows      []Row
	deleted   []bool
	blockNums []uint64
	// sign, for a signed append table, is -1 for cancel rows (1 when 0).
	sign int8
}

type historyRow struct {
	table    string
	pk       map[string]string
	blockNum uint64
	prev     Row
}

// keyedTableResult is what a flush writes for a keyed table, and journals.
type keyedTableResult struct {
	write        rowsWrite
	history      []historyRow
	beforeImages []beforeImage
}

// evaluateKeyedTable reads the current rows of a keyed table's pending operations and applies them.
func (d ClickhouseDialect) evaluateKeyedTable(ctx context.Context, l *Loader, table *TableInfo, ops *OrderedMap[string, *Operation]) (keyedTableResult, error) {
	result := keyedTableResult{write: rowsWrite{table: table}}

	// Keys are grouped in the form ClickHouse stores them: "01" and "1" in an integer column are one
	// row, whose operations apply in order.
	var keys []string
	opsByKey := map[string][]*Operation{}
	for opPair := ops.Oldest(); opPair != nil; opPair = opPair.Next() {
		op := opPair.Value
		if err := checkPrimaryKeyUnchanged(op); err != nil {
			return result, err
		}
		id, err := canonicalRowID(table, op.primaryKey)
		if err != nil {
			return result, err
		}
		if _, seen := opsByKey[id]; !seen {
			keys = append(keys, id)
		}
		opsByKey[id] = append(opsByKey[id], op)
	}
	firstOps := make([]*Operation, len(keys))
	for i, id := range keys {
		keyOps := opsByKey[id]
		slices.SortFunc(keyOps, func(a, b *Operation) int { return cmp.Compare(a.ordinal, b.ordinal) })
		firstOps[i] = keyOps[0]
	}

	current, err := d.readCurrentRows(ctx, l, table, firstOps)
	if err != nil {
		return result, fmt.Errorf("reading current rows of %q: %w", table.name, err)
	}

	for _, id := range keys {
		keyOps := opsByKey[id]
		pk, err := canonicalKey(table, keyOps[0].primaryKey)
		if err != nil {
			return result, err
		}
		before := current[id]
		state, changed := before, false
		historyBlocks := map[uint64]bool{}
		for _, op := range keyOps {
			row, outcome, err := evaluateOperation(op, state)
			if err != nil {
				return result, err
			}
			if outcome == rowUnchanged {
				continue
			}
			if outcome == rowWritten && state == nil {
				if err := d.fillUnsetColumns(table, row); err != nil {
					return result, err
				}
			}
			// One history row per block, which can hold several operations on the row.
			if d.stateful.reorgs && op.reversibleBlockNum != nil && !historyBlocks[*op.reversibleBlockNum] {
				historyBlocks[*op.reversibleBlockNum] = true
				result.history = append(result.history, historyRow{table: table.name, pk: pk, blockNum: *op.reversibleBlockNum, prev: state})
			}
			if outcome == rowDeleted {
				row = nil
			}
			state, changed = row, true
		}
		if !changed {
			continue
		}
		deleted := state == nil
		if deleted {
			if before == nil {
				continue
			}
			state = keyRow(pk)
		}
		result.write.rows = append(result.write.rows, state)
		result.write.deleted = append(result.write.deleted, deleted)
		result.beforeImages = append(result.beforeImages, beforeImage{Key: pk, Row: before})
	}
	return result, nil
}

// fillUnsetColumns gives a new row's unset columns the value ClickHouse stores for them, so later
// operations in the batch see what a later flush would read back.
func (d ClickhouseDialect) fillUnsetColumns(table *TableInfo, row Row) error {
	for name, c := range table.columnsByName {
		if _, set := row[name]; set {
			continue
		}
		if value, found := d.stateful.defaults[table.name][name]; found {
			row[name] = &value
			continue
		}
		stored, err := storedZeroValue(c)
		if err != nil {
			return fmt.Errorf("table %q column %q: %w", table.name, name, err)
		}
		row[name] = stored
	}
	return nil
}

// storedZeroValue is what ClickHouse stores for a column left out: NULL, or the type's zero value.
func storedZeroValue(c *ColumnInfo) (*string, error) {
	v, err := statefulColumnValue(c, nil)
	if err != nil {
		return nil, err
	}
	return formatClickhouseValue(v, c)
}

// checkPrimaryKeyUnchanged refuses an operation changing a primary key column's value.
func checkPrimaryKeyUnchanged(op *Operation) error {
	for name, value := range op.primaryKey {
		if field, found := op.data[name]; found && (field.UpdateOp != UpdateOpSet || field.Value != value) {
			return fmt.Errorf("row %s: changing primary key column %q (%s %q) is not supported in ClickHouse stateful mode", op, name, updateOpName(field.UpdateOp), field.Value)
		}
	}
	return nil
}

func (d ClickhouseDialect) insertHistory(ctx context.Context, l *Loader, history []historyRow, seq uint64) error {
	if len(history) == 0 {
		return nil
	}
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	writeCtx := d.sinkWrite(ctx)
	tx, err := l.DB.BeginTx(writeCtx, nil)
	if err != nil {
		return fmt.Errorf("beginning history batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // nothing to roll back once committed
	batch, err := tx.PrepareContext(writeCtx, fmt.Sprintf("INSERT INTO %s (table_name, pk, block_num, prev_value, seq)", d.historyTableIdentifier()))
	if err != nil {
		return fmt.Errorf("preparing history insert: %w", err)
	}
	defer batch.Close()
	for _, h := range history {
		var prev *string
		if h.prev != nil {
			encoded, err := json.Marshal(h.prev)
			if err != nil {
				return fmt.Errorf("encoding history row: %w", err)
			}
			s := string(encoded)
			prev = &s
		}
		if _, err := batch.ExecContext(writeCtx, h.table, primaryKeyToJSON(h.pk), h.blockNum, prev, seq); err != nil {
			return fmt.Errorf("adding history row: %w", err)
		}
	}
	// Checked again, as building the batch can take a while.
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("inserting history: %w", err)
	}
	return nil
}

func keyRow(primaryKey map[string]string) Row {
	row := make(Row, len(primaryKey))
	for k, v := range primaryKey {
		row[k] = &v
	}
	return row
}

func sortedKeyColumns(table *TableInfo) []*ColumnInfo {
	keyColumns := slices.Clone(table.primaryColumns)
	slices.SortFunc(keyColumns, compareColumnNames)
	return keyColumns
}

// canonicalRowID is the row ID of a primary key in its canonical form (see canonicalKey).
func canonicalRowID(table *TableInfo, primaryKey map[string]string) (string, error) {
	key, err := canonicalKey(table, primaryKey)
	if err != nil {
		return "", err
	}
	return createRowUniqueID(key), nil
}

// canonicalKey is a primary key with its values written as ClickHouse gives them back.
func canonicalKey(table *TableInfo, primaryKey map[string]string) (map[string]string, error) {
	key := map[string]string{}
	for _, c := range sortedKeyColumns(table) {
		raw := primaryKey[c.name]
		v, err := statefulColumnValue(c, &raw)
		if err != nil {
			return nil, fmt.Errorf("key column %q value %q: %w", c.name, raw, err)
		}
		s, err := formatClickhouseValue(v, c)
		if err != nil {
			return nil, fmt.Errorf("key column %q: %w", c.name, err)
		}
		if s != nil {
			key[c.name] = *s
		}
	}
	return key, nil
}

// readCurrentRows reads the current rows of the operations' keys, by canonical row ID. Keys go in
// an external table, which max_query_size does not cap, and LIMIT 1 BY picks each row's latest
// version without the cost of FINAL.
func (d ClickhouseDialect) readCurrentRows(ctx context.Context, l *Loader, table *TableInfo, ops []*Operation) (map[string]Row, error) {
	keyColumns := sortedKeyColumns(table)

	extColumns := make([]func(*ext.Table) error, 0, len(keyColumns))
	keyNames := make([]string, len(keyColumns))
	for i, c := range keyColumns {
		extColumns = append(extColumns, ext.Column(c.name, column.Type(unwrapColumnType(c.databaseTypeName))))
		keyNames[i] = c.escapedName
	}
	keysTable, err := ext.NewTable("_sink_keys", extColumns...)
	if err != nil {
		return nil, fmt.Errorf("creating keys table: %w", err)
	}
	for _, op := range ops {
		values := make([]any, len(keyColumns))
		for i, c := range keyColumns {
			raw := op.primaryKey[c.name]
			v, err := statefulColumnValue(c, &raw)
			if err != nil {
				return nil, fmt.Errorf("key column %q value %q: %w", c.name, raw, err)
			}
			values[i] = v
		}
		if err := keysTable.Append(values...); err != nil {
			return nil, fmt.Errorf("adding key: %w", err)
		}
	}

	columns := sortedColumns(table)
	selected := make([]string, len(columns))
	for i, c := range columns {
		selected[i] = c.escapedName
	}
	query := fmt.Sprintf("SELECT %[1]s, %[2]s, %[3]s FROM %[4]s WHERE (%[5]s) IN (SELECT %[5]s FROM _sink_keys) ORDER BY %[2]s DESC LIMIT 1 BY %[5]s",
		strings.Join(selected, ", "), statefulVersionColumn, statefulDeletedColumn, table.identifier, strings.Join(keyNames, ", "))

	rows, err := l.QueryContext(clickhouse.Context(ctx, clickhouse.WithExternalTable(keysTable)), query)
	if err != nil {
		return nil, fmt.Errorf("querying current rows: %w", err)
	}
	defer rows.Close()

	out := map[string]Row{}
	for rows.Next() {
		dest := make([]any, len(columns)+2)
		for i, c := range columns {
			dest[i] = reflect.New(c.scanType).Interface()
		}
		var version uint64
		var deleted uint8
		dest[len(columns)] = &version
		dest[len(columns)+1] = &deleted
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		if deleted == 1 {
			continue
		}

		row := make(Row, len(columns))
		for i, c := range columns {
			value, err := formatClickhouseValue(reflect.ValueOf(dest[i]).Elem().Interface(), c)
			if err != nil {
				return nil, fmt.Errorf("column %q: %w", c.name, err)
			}
			row[c.name] = value
		}
		key := make(map[string]string, len(keyColumns))
		for _, c := range keyColumns {
			if row[c.name] != nil {
				key[c.name] = *row[c.name]
			}
		}
		out[createRowUniqueID(key)] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading current rows: %w", err)
	}
	return out, nil
}

// insertRows writes full rows with the given version and the table's sink-managed columns.
func (d ClickhouseDialect) insertRows(ctx context.Context, l *Loader, w rowsWrite, version uint64) error {
	if len(w.rows) == 0 {
		return nil
	}
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	table := w.table
	role := d.stateful.roles[table.name]
	withBlockNum := role != tableRoleKeyed && d.stateful.blockNumColumn[table.name]
	sign := w.sign
	if sign == 0 {
		sign = 1
	}

	columns := sortedColumns(table)
	names := make([]string, 0, len(columns)+3)
	for _, c := range columns {
		names = append(names, c.escapedName)
	}
	names = append(names, statefulVersionColumn)
	switch role {
	case tableRoleKeyed:
		names = append(names, statefulDeletedColumn)
	case tableRoleAppendSigned:
		names = append(names, statefulSignColumn)
	}
	if withBlockNum {
		names = append(names, statefulBlockNumColumn)
	}

	startAt := time.Now()
	writeCtx := d.sinkWrite(ctx)
	tx, err := l.DB.BeginTx(writeCtx, nil)
	if err != nil {
		return fmt.Errorf("beginning batch for %q: %w", table.name, err)
	}
	defer func() { _ = tx.Rollback() }() // nothing to roll back once committed

	batch, err := tx.PrepareContext(writeCtx, fmt.Sprintf("INSERT INTO %s (%s)", table.identifier, strings.Join(names, ", ")))
	if err != nil {
		return fmt.Errorf("preparing insert into %q: %w", table.name, err)
	}
	defer batch.Close()
	for i, row := range w.rows {
		values := make([]any, 0, len(names))
		for _, c := range columns {
			value, set := row[c.name]
			if !set {
				if def, found := d.stateful.defaults[table.name][c.name]; found {
					value = &def
				}
			}
			v, err := statefulColumnValue(c, value)
			if err != nil {
				return fmt.Errorf("table %q column %q: %w", table.name, c.name, err)
			}
			values = append(values, v)
		}
		values = append(values, version)
		switch role {
		case tableRoleKeyed:
			var flag uint8
			if w.deleted[i] {
				flag = 1
			}
			values = append(values, flag)
		case tableRoleAppendSigned:
			values = append(values, sign)
		}
		if withBlockNum {
			values = append(values, w.blockNums[i])
		}
		if _, err := batch.ExecContext(writeCtx, values...); err != nil {
			return fmt.Errorf("adding row to %q: %w", table.name, err)
		}
	}
	// Checked again, as building the batch can take a while.
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("inserting into %q: %w", table.name, err)
	}
	QueryExecutionDuration.AddInt64(time.Since(startAt).Nanoseconds(), "normal")
	return nil
}

func sortedColumns(table *TableInfo) []*ColumnInfo {
	return slices.SortedFunc(maps.Values(table.columnsByName), compareColumnNames)
}

func compareColumnNames(a, b *ColumnInfo) int {
	return strings.Compare(a.name, b.name)
}
