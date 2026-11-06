package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	sink "github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
)

// journalEntry is a flush or revert as recorded in the journal table.
type journalEntry struct {
	moduleHash string
	seq        uint64
	// baseCursor is the stored cursor it started from, "" if none.
	baseCursor string
	// beforeImages holds, per keyed table, the rows a flush changes as they were, or the rows a
	// revert writes back.
	beforeImages map[string][]beforeImage
	appendTables []string
	// revert is set on a revert's entry, which is completed rather than rolled back if cut short.
	revert *journalRevert
}

// beforeImage is a keyed row as it was; Row is nil when it did not exist.
type beforeImage struct {
	Key map[string]string `json:"key"`
	Row Row               `json:"row"`
}

type journalRevert struct {
	LastValidBlock uint64 `json:"last_valid_block"`
	Cursor         string `json:"cursor"`
	// KeptChanges is set when the undo keeps pending changes, flushed after the revert.
	KeptChanges bool `json:"kept_changes,omitempty"`
}

type journalPayload struct {
	BeforeImages map[string][]beforeImage `json:"before_images"`
	AppendTables []string                 `json:"append_tables"`
	Revert       *journalRevert           `json:"revert,omitempty"`
}

// RecoverClickhouseStateful finishes what a previous run left half done and sets the next journal
// sequence. It runs once the writer lock is held, before the stored cursor is read.
func (l *Loader) RecoverClickhouseStateful(ctx context.Context) error {
	d, ok := l.dialect.(*ClickhouseDialect)
	if !ok || d.stateful == nil {
		return nil
	}
	return d.recover(synchronousInserts(ctx), l)
}

// writeCursorDirectly writes a module's cursor outside a flush.
func (l *Loader) writeCursorDirectly(ctx context.Context, moduleHash string, cursor *sink.Cursor) error {
	return l.UpdateCursor(ctx, nil, moduleHash, cursor)
}

// newJournalEntry starts the journal entry of a flush or a revert.
func (d ClickhouseDialect) newJournalEntry(l *Loader, outputModuleHash string) *journalEntry {
	entry := &journalEntry{
		moduleHash:   outputModuleHash,
		seq:          d.stateful.nextSeq,
		beforeImages: map[string][]beforeImage{},
	}
	if l.expectedCursor != nil {
		entry.baseCursor = *l.expectedCursor
	}
	return entry
}

// resolveUncommitted handles a previous flush or revert whose cursor write failed. If the cursor
// was written after all (a lost reply), done is true and the retried batch must not be applied
// again; otherwise the entry is resolved.
func (d ClickhouseDialect) resolveUncommitted(ctx context.Context, l *Loader) (done bool, err error) {
	st := d.stateful
	if st.uncommitted == nil {
		return false, nil
	}
	if st.uncommitted.revert != nil && l.reverting {
		// The kept changes of an undo, flushed right after its revert: they commit together.
		st.uncommitted = nil
		return false, nil
	}
	if err := d.stopSinkWrites(ctx, l); err != nil {
		return false, err
	}
	stored, err := storedCursor(ctx, l, st.uncommitted.moduleHash)
	if err != nil {
		return false, err
	}
	if stored != st.uncommitted.baseCursor {
		st.uncommitted = nil
		return true, nil
	}
	if err := d.resolve(ctx, l, st.uncommitted); err != nil {
		return false, fmt.Errorf("resolving the previous attempt: %w", err)
	}
	st.uncommitted = nil
	return false, nil
}

// resolve finishes an entry whose cursor was not written: a flush is rolled back, a revert is
// completed.
func (d ClickhouseDialect) resolve(ctx context.Context, l *Loader, entry *journalEntry) error {
	if entry.revert == nil {
		l.logger.Info("rolling back a flush whose cursor was not written", zap.Uint64("seq", entry.seq), zap.String("module_hash", entry.moduleHash))
		return d.rollback(ctx, l, entry)
	}
	if entry.revert.KeptChanges {
		// Kept changes come from blocks after the stored cursor, so the revert wrote nothing, and
		// its cursor would skip them. The stream resumes from the stored cursor instead.
		l.logger.Info("dropping a revert whose kept changes were not written", zap.Uint64("seq", entry.seq), zap.Uint64("last_valid_block", entry.revert.LastValidBlock))
		return nil
	}

	l.logger.Info("completing a revert whose cursor was not written", zap.Uint64("seq", entry.seq), zap.Uint64("last_valid_block", entry.revert.LastValidBlock))
	if err := d.applyRevert(ctx, l, entry); err != nil {
		return err
	}
	cursor, err := sink.NewCursor(entry.revert.Cursor)
	if err != nil {
		return fmt.Errorf("decoding the revert's cursor: %w", err)
	}
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	return l.writeCursorDirectly(d.sinkWrite(ctx), entry.moduleHash, cursor)
}

// flushCommitted is called by the loader once the cursor of the last flush or revert is written.
func (d ClickhouseDialect) flushCommitted() {
	if d.stateful != nil {
		d.stateful.uncommitted = nil
	}
}

// pruneJournalAndHistory periodically removes old journal entries and the history of final blocks.
func (d ClickhouseDialect) pruneJournalAndHistory(ctx context.Context, l *Loader, seq, lastFinalBlock uint64) {
	if seq%journalPruneInterval == 0 {
		if err := d.lightweightDelete(ctx, l, d.journalTableIdentifier(), fmt.Sprintf("seq < %d", seq-journalKeptEntries)); err != nil {
			l.logger.Warn("pruning the journal failed, will retry", zap.Error(err))
		}
	}
	if d.stateful.reorgs && seq%historyPruneInterval == 0 && lastFinalBlock > 0 {
		if err := d.lightweightDelete(ctx, l, d.historyTableIdentifier(), fmt.Sprintf("block_num <= %d", lastFinalBlock)); err != nil {
			l.logger.Warn("pruning the history failed, will retry", zap.Error(err))
		}
	}
}

// writeJournal stores a journal entry and makes it the uncommitted one. If storing fails, its
// sequence is reused: a copy stored despite the error is identical.
func (d ClickhouseDialect) writeJournal(ctx context.Context, l *Loader, entry *journalEntry) error {
	if err := l.checkWriterLease(ctx, d); err != nil {
		return err
	}
	payload, err := json.Marshal(journalPayload{BeforeImages: entry.beforeImages, AppendTables: entry.appendTables, Revert: entry.revert})
	if err != nil {
		return fmt.Errorf("encoding journal payload: %w", err)
	}
	_, err = l.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (module_hash, seq, base_cursor, payload) VALUES (?, ?, ?, ?)", d.journalTableIdentifier()),
		entry.moduleHash, entry.seq, entry.baseCursor, string(payload))
	if err != nil {
		return err
	}
	d.stateful.nextSeq = entry.seq + 1
	d.stateful.uncommitted = entry
	return nil
}

// checkStoredCursor fails with ErrCursorConflict if the module's stored cursor is not the one this
// sink last read or wrote.
func checkStoredCursor(ctx context.Context, l *Loader) error {
	if l.expectedCursor == nil {
		return nil
	}
	stored, err := storedCursor(ctx, l, l.activeModuleHash)
	if err != nil {
		return err
	}
	if stored != *l.expectedCursor {
		return ErrCursorConflict
	}
	return nil
}

func storedCursor(ctx context.Context, l *Loader, moduleHash string) (string, error) {
	rows, err := l.allCursorRows(ctx)
	if err != nil {
		return "", fmt.Errorf("reading the stored cursor: %w", err)
	}
	return rows[moduleHash].Cursor, nil
}

// recover resolves, from the latest, the entries cut short: those whose module's stored cursor is
// still the one they started from. A revert ends the search. Sequences are shared across modules,
// as a new module version takes over the previous one's tables.
func (d ClickhouseDialect) recover(ctx context.Context, l *Loader) error {
	if err := d.stopSinkWrites(ctx, l); err != nil {
		return err
	}
	entries, err := d.readLatestJournalEntries(ctx, l, 10)
	if err != nil {
		return err
	}

	d.stateful.nextSeq = 1
	if len(entries) > 0 {
		d.stateful.nextSeq = entries[0].seq + 1
	}
	for _, entry := range entries {
		stored, err := storedCursor(ctx, l, entry.moduleHash)
		if err != nil {
			return err
		}
		if stored != entry.baseCursor {
			return nil
		}
		if err := d.resolve(ctx, l, entry); err != nil {
			return err
		}
		if entry.revert != nil {
			return nil
		}
	}
	return nil
}

// readLatestJournalEntries reads the latest journal entries, newest first.
func (d ClickhouseDialect) readLatestJournalEntries(ctx context.Context, l *Loader, limit int) ([]*journalEntry, error) {
	rows, err := l.QueryContext(ctx,
		fmt.Sprintf("SELECT seq, module_hash, base_cursor, payload FROM %s ORDER BY seq DESC LIMIT %d", d.journalTableIdentifier(), limit))
	if err != nil {
		return nil, fmt.Errorf("reading the journal: %w", err)
	}
	defer rows.Close()

	var entries []*journalEntry
	for rows.Next() {
		var entry journalEntry
		var payload string
		if err := rows.Scan(&entry.seq, &entry.moduleHash, &entry.baseCursor, &payload); err != nil {
			return nil, fmt.Errorf("reading the journal: %w", err)
		}
		var decoded journalPayload
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			return nil, fmt.Errorf("decoding journal entry %d: %w", entry.seq, err)
		}
		entry.beforeImages, entry.appendTables, entry.revert = decoded.BeforeImages, decoded.AppendTables, decoded.Revert
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the journal: %w", err)
	}
	return entries, nil
}

// rollback undoes a flush that may be partly written. Running it again gives the same result.
func (d ClickhouseDialect) rollback(ctx context.Context, l *Loader, entry *journalEntry) error {
	version := 2*entry.seq + 1
	if err := d.writeBeforeImages(ctx, l, entry, version); err != nil {
		return err
	}

	for _, tableName := range entry.appendTables {
		table, found := l.tables[tableName]
		if !found {
			return fmt.Errorf("journal entry %d: unknown table %q", entry.seq, tableName)
		}
		// The flush's rows, and cancel rows of an earlier run of this rollback.
		where := fmt.Sprintf("%s IN (%d, %d)", statefulVersionColumn, 2*entry.seq, 2*entry.seq+1)
		if err := d.removeAppendRows(ctx, l, table, d.stateful.roles[tableName], where, version); err != nil {
			return fmt.Errorf("removing rows of %q: %w", tableName, err)
		}
	}

	if d.stateful.reorgs {
		if err := d.lightweightDelete(ctx, l, d.historyTableIdentifier(), fmt.Sprintf("seq = %d", entry.seq)); err != nil {
			return fmt.Errorf("removing history: %w", err)
		}
	}
	return nil
}

// writeBeforeImages writes an entry's before-images back with version.
func (d ClickhouseDialect) writeBeforeImages(ctx context.Context, l *Loader, entry *journalEntry, version uint64) error {
	for tableName, images := range entry.beforeImages {
		table, found := l.tables[tableName]
		if !found {
			return fmt.Errorf("journal entry %d: unknown table %q", entry.seq, tableName)
		}
		write := rowsWrite{table: table, rows: make([]Row, len(images)), deleted: make([]bool, len(images))}
		for i, image := range images {
			if image.Row == nil {
				write.rows[i] = keyRow(image.Key)
				write.deleted[i] = true
			} else {
				write.rows[i] = image.Row
			}
		}
		if err := d.insertRows(ctx, l, write, version); err != nil {
			return fmt.Errorf("writing back rows of %q: %w", tableName, err)
		}
	}
	return nil
}

// sinkWrite tags a write with a query ID that stopSinkWrites can find.
func (d ClickhouseDialect) sinkWrite(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithQueryID(d.sinkWritePrefix()+uuid.NewString()))
}

func (d ClickhouseDialect) sinkWritePrefix() string {
	return "substreams-sink:" + d.schemaName + ":"
}

// stopSinkWrites kills the sink's writes still running in ClickHouse, from this process or a
// previous one, and waits for them to stop. A write given up on (a timeout, a lost connection) must
// not land after a rollback.
func (d ClickhouseDialect) stopSinkWrites(ctx context.Context, l *Loader) error {
	clusterClause := ""
	if d.cluster != "" {
		clusterClause = " ON CLUSTER " + EscapeIdentifier(d.cluster)
	}
	if _, err := l.ExecContext(ctx, "KILL QUERY"+clusterClause+" WHERE startsWith(query_id, ?) SYNC", d.sinkWritePrefix()); err != nil {
		return fmt.Errorf("stopping writes still running: %w", err)
	}
	return nil
}
