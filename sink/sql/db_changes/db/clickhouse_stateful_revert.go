package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// revertStateful reverts the blocks after lastValidBlock: keyed rows are written back from the
// history table and append rows of reverted blocks are removed. A revert cut short is completed
// from its journal entry; each step is idempotent.
func (d ClickhouseDialect) revertStateful(ctx context.Context, l *Loader, lastValidBlock uint64) error {
	ctx = synchronousInserts(ctx)
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	if _, err := d.resolveUncommitted(ctx, l); err != nil {
		return err
	}
	if err := checkStoredCursor(ctx, l); err != nil {
		return err
	}

	entry := d.newJournalEntry(l, l.activeModuleHash)
	entry.revert = &journalRevert{LastValidBlock: lastValidBlock, KeptChanges: l.keepsPendingChanges(lastValidBlock)}
	if l.activeCursor != nil {
		entry.revert.Cursor = l.activeCursor.String()
	}
	// Journaled, as completing the revert cannot re-read history it may have partly deleted.
	restores, err := d.readRevertRestores(ctx, l, lastValidBlock)
	if err != nil {
		return err
	}
	entry.beforeImages = restores
	if err := d.writeJournal(ctx, l, entry); err != nil {
		return fmt.Errorf("writing journal: %w", err)
	}
	return d.applyRevert(ctx, l, entry)
}

// keepsPendingChanges reports whether an undo to lastValidBlock keeps pending changes.
func (l *Loader) keepsPendingChanges(lastValidBlock uint64) bool {
	for tablePair := l.entries.Oldest(); tablePair != nil; tablePair = tablePair.Next() {
		for opPair := tablePair.Value.Oldest(); opPair != nil; opPair = opPair.Next() {
			if block := opPair.Value.reversibleBlockNum; block == nil || *block <= lastValidBlock {
				return true
			}
		}
	}
	return false
}

// readRevertRestores reads, per table, the keyed rows changed after lastValidBlock as they were
// before it.
func (d ClickhouseDialect) readRevertRestores(ctx context.Context, l *Loader, lastValidBlock uint64) (map[string][]beforeImage, error) {
	rows, err := l.QueryContext(ctx, fmt.Sprintf(
		"SELECT table_name, pk, prev_value FROM %s FINAL WHERE block_num > ? ORDER BY block_num",
		d.historyTableIdentifier()), lastValidBlock)
	if err != nil {
		return nil, fmt.Errorf("reading history: %w", err)
	}
	defer rows.Close()

	restores := map[string][]beforeImage{}
	seen := map[string]map[string]bool{}
	for rows.Next() {
		var tableName, pk string
		var prev sql.NullString
		if err := rows.Scan(&tableName, &pk, &prev); err != nil {
			return nil, fmt.Errorf("reading history: %w", err)
		}
		if seen[tableName] == nil {
			seen[tableName] = map[string]bool{}
		}
		if seen[tableName][pk] {
			continue // the earliest reverted block holds the state before the fork
		}
		seen[tableName][pk] = true
		key, err := jsonToPrimaryKey(pk)
		if err != nil {
			return nil, fmt.Errorf("decoding history primary key %q: %w", pk, err)
		}
		var row Row
		if prev.Valid {
			if err := json.Unmarshal([]byte(prev.String), &row); err != nil {
				return nil, fmt.Errorf("decoding history row: %w", err)
			}
		}
		restores[tableName] = append(restores[tableName], beforeImage{Key: key, Row: row})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading history: %w", err)
	}
	return restores, nil
}

// applyRevert does the writes of a revert's journal entry.
func (d ClickhouseDialect) applyRevert(ctx context.Context, l *Loader, entry *journalEntry) error {
	version := 2 * entry.seq
	if err := d.writeBeforeImages(ctx, l, entry, version); err != nil {
		return err
	}

	lastValidBlock := entry.revert.LastValidBlock
	for tableName, role := range d.stateful.roles {
		if role == tableRoleKeyed {
			continue
		}
		where := fmt.Sprintf("%s > %d", statefulBlockNumColumn, lastValidBlock)
		if err := d.removeAppendRows(ctx, l, l.tables[tableName], role, where, version); err != nil {
			return fmt.Errorf("reverting rows of %q: %w", tableName, err)
		}
	}

	if err := d.lightweightDelete(ctx, l, d.historyTableIdentifier(), fmt.Sprintf("block_num > %d", lastValidBlock)); err != nil {
		return fmt.Errorf("pruning reverted history: %w", err)
	}
	return nil
}

// removeAppendRows removes an append table's rows matching where: deleted, or in a signed table,
// cancelled by -1 rows.
func (d ClickhouseDialect) removeAppendRows(ctx context.Context, l *Loader, table *TableInfo, role tableRole, where string, version uint64) error {
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	if role != tableRoleAppendSigned {
		return d.lightweightDelete(ctx, l, table.identifier, where)
	}

	columns := sortedColumns(table)
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.escapedName
	}
	if d.stateful.blockNumColumn[table.name] {
		names = append(names, statefulBlockNumColumn)
	}
	list := strings.Join(names, ", ")
	query := fmt.Sprintf(`
		INSERT INTO %[1]s (%[2]s, %[3]s, %[4]s)
		SELECT %[2]s, %[5]d, -1
		FROM (
			SELECT %[2]s, sum(%[4]s) AS live
			FROM %[1]s WHERE %[6]s
			GROUP BY %[2]s
			HAVING live > 0
		)
		ARRAY JOIN range(toUInt64(live)) AS _copy`,
		table.identifier, list, statefulVersionColumn, statefulSignColumn, version, where)
	_, err := l.ExecContext(d.sinkWrite(ctx), query)
	return err
}

func (d ClickhouseDialect) lightweightDelete(ctx context.Context, l *Loader, tableIdentifier, where string) error {
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	ctx = clickhouse.Context(d.sinkWrite(ctx), clickhouse.WithSettings(clickhouse.Settings{"lightweight_deletes_sync": 2}))
	_, err := l.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", tableIdentifier, where))
	return err
}
