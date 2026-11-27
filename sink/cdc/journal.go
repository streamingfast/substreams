package cdc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Names of the flags that make updates and deletes safe, used in user-facing errors. They live here so a
// rename of the sink flags is a one-line change.
const (
	FlagFinalBlocksOnly = "--final-blocks-only"
	FlagUndoBufferSize  = "--undo-buffer-size"
)

// IrreversibleError is returned when a reorg needs to revert an update or a delete: with no before-image,
// the previous row cannot be restored.
type IrreversibleError struct {
	Event          Event
	LastValidBlock uint64
}

func (e *IrreversibleError) Error() string {
	return fmt.Sprintf(
		"cannot revert %s event %q (module %q, table %q, block #%d) after an undo to block #%d: no before-image is available to restore the previous row. "+
			"Only inserts can be reverted, run with %s (final blocks only) or %s N (pseudo-final blocks, N blocks behind head) so updates and deletes are only emitted once irreversible",
		opName(e.Event.Op), e.Event.ID, e.Event.Source.Module, e.Event.Source.Table, e.Event.Source.Block.Number, e.LastValidBlock,
		FlagFinalBlocksOnly, FlagUndoBufferSize,
	)
}

func opName(op Op) string {
	switch op {
	case OpUpdate:
		return "update"
	case OpDelete:
		return "delete"
	default:
		return "create"
	}
}

// Journal keeps the events emitted for reversible blocks, in emission order, so that a chain reorg can be
// answered with compensation events. It is not safe for concurrent use.
type Journal struct {
	events []Event
}

func NewJournal() *Journal { return &Journal{} }

// Len is the number of journaled events.
func (j *Journal) Len() int { return len(j.events) }

// Events returns a copy of the journaled events, in emission order.
func (j *Journal) Events() []Event { return slices.Clone(j.events) }

// Add journals events in emission order. An event whose id is already journaled (a block replayed after a
// restart) is skipped.
func (j *Journal) Add(events ...Event) {
	for _, event := range events {
		if slices.ContainsFunc(j.events, func(known Event) bool { return known.ID == event.ID }) {
			continue
		}
		j.events = append(j.events, event)
	}
}

// Prune drops the events of blocks at or below the final block height, they can't be reverted anymore.
func (j *Journal) Prune(finalBlockHeight uint64) {
	j.events = slices.DeleteFunc(j.events, func(event Event) bool {
		return event.Source.Block.Number <= finalBlockHeight
	})
}

// Compensations returns the events that revert everything emitted for blocks above lastValidBlock, in
// reverse emission order. A journaled create becomes a delete tombstone (same key, no `after`, `revert_of` the
// original id, original block), a journaled update or delete fails with an [IrreversibleError]. The journal
// is left untouched, call [Journal.Discard] once the compensations are emitted.
func (j *Journal) Compensations(lastValidBlock uint64, cursor string) ([]Event, error) {
	var out []Event
	for _, event := range slices.Backward(j.events) {
		if event.Source.Block.Number <= lastValidBlock {
			continue
		}

		if event.Op != OpCreate {
			return nil, &IrreversibleError{Event: event, LastValidBlock: lastValidBlock}
		}

		tombstone := Event{
			ID:       event.ID + ":revert",
			Op:       OpDelete,
			Key:      event.Key,
			RevertOf: event.ID,
			Source:   event.Source,
		}
		tombstone.Source.Final = false
		tombstone.Source.Cursor = cursor
		out = append(out, tombstone)
	}
	return out, nil
}

// Discard drops the events of blocks above lastValidBlock.
func (j *Journal) Discard(lastValidBlock uint64) {
	j.events = slices.DeleteFunc(j.events, func(event Event) bool {
		return event.Source.Block.Number > lastValidBlock
	})
}

const journalVersion = 1

type journalFile struct {
	Version int `json:"version"`
	// Cursor is the cursor the journal was saved at.
	Cursor string  `json:"cursor"`
	Events []Event `json:"events"`
}

// JournalPath derives the journal file from the sink state (cursor) file.
func JournalPath(stateFile string) string {
	if stateFile == "" {
		return ""
	}
	return stateFile + ".journal"
}

// Save writes the journal, tagged with cursor, using a temp file and a rename. It does nothing when path is empty.
//
// Save it before writing the cursor file: a crash in between leaves a journal ahead of the cursor, which is
// harmless since replayed events are deduplicated by [Journal.Add], while the reverse would lose events a
// later undo needs to compensate.
func (j *Journal) Save(path string, cursor string) error {
	if path == "" {
		return nil
	}

	data, err := json.Marshal(journalFile{Version: journalVersion, Cursor: cursor, Events: j.events})
	if err != nil {
		return fmt.Errorf("encode journal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".journal_*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// LoadJournal reads a journal saved by [Journal.Save] and the cursor it was saved at. A missing file, or an
// empty path, gives an empty journal.
func LoadJournal(path string) (*Journal, string, error) {
	if path == "" {
		return NewJournal(), "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewJournal(), "", nil
		}
		return nil, "", err
	}

	var file journalFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&file); err != nil {
		return nil, "", fmt.Errorf("decode journal %q: %w", path, err)
	}
	if file.Version != journalVersion {
		return nil, "", fmt.Errorf("journal %q has version %d, expected %d", path, file.Version, journalVersion)
	}

	return &Journal{events: file.Events}, file.Cursor, nil
}
