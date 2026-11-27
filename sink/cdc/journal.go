package cdc

import (
	"errors"
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"

	pbcdc "github.com/streamingfast/substreams/pb/sf/substreams/sink/cdc/v1"
	"github.com/streamingfast/substreams/sink"
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
			"Only inserts can be reverted, run with --%s (final blocks only) or --%s N (pseudo-final blocks, N blocks behind head) so updates and deletes are only emitted once irreversible",
		opName(e.Event.Op), e.Event.ID, e.Event.Source.Module, e.Event.Source.Table, e.Event.Source.Block.Number, e.LastValidBlock,
		sink.FlagFinalBlocksOnly, sink.FlagUndoBufferSize,
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

// journalBlock holds the events emitted for one block, in emission order.
type journalBlock struct {
	block  Block
	events []Event
}

// Journal keeps the events emitted for reversible blocks so that a chain reorg can be answered with
// compensation events. Events are paged by block, oldest block first, so pruning finalized blocks drops
// pages from the front and an undo drops them from the back. It is not safe for concurrent use.
type Journal struct {
	blocks []journalBlock
	// ids holds the id of every journaled event, to skip a block replayed after a restart.
	ids map[string]struct{}
}

func NewJournal() *Journal { return &Journal{ids: map[string]struct{}{}} }

// Len is the number of journaled events.
func (j *Journal) Len() int { return len(j.ids) }

// Events returns the journaled events, in emission order.
func (j *Journal) Events() []Event {
	var out []Event
	for _, page := range j.blocks {
		out = append(out, page.events...)
	}
	return out
}

// Add journals events in emission order. An event whose id is already journaled (a block replayed after a
// restart) is skipped.
func (j *Journal) Add(events ...Event) {
	for _, event := range events {
		if _, found := j.ids[event.ID]; found {
			continue
		}
		j.ids[event.ID] = struct{}{}

		// Events of one block are contiguous, so only the last page can be the one to extend.
		if last := len(j.blocks) - 1; last >= 0 && j.blocks[last].block.ID == event.Source.Block.ID && j.blocks[last].block.Number == event.Source.Block.Number {
			j.blocks[last].events = append(j.blocks[last].events, event)
			continue
		}
		j.blocks = append(j.blocks, journalBlock{block: event.Source.Block, events: []Event{event}})
	}
}

// Prune drops the events of blocks at or below the final block height, they can't be reverted anymore.
func (j *Journal) Prune(finalBlockHeight uint64) {
	drop := 0
	for drop < len(j.blocks) && j.blocks[drop].block.Number <= finalBlockHeight {
		j.forget(j.blocks[drop])
		drop++
	}
	j.blocks = j.blocks[drop:]
}

// Discard drops the events of blocks above lastValidBlock.
func (j *Journal) Discard(lastValidBlock uint64) {
	keep := len(j.blocks)
	for keep > 0 && j.blocks[keep-1].block.Number > lastValidBlock {
		keep--
		j.forget(j.blocks[keep])
	}
	j.blocks = j.blocks[:keep]
}

func (j *Journal) forget(page journalBlock) {
	for _, event := range page.events {
		delete(j.ids, event.ID)
	}
}

// Compensations returns the events that revert everything emitted for blocks above lastValidBlock, in
// reverse emission order. A journaled create becomes a delete tombstone (id `<original id>:revert`, same key,
// no `after`, `revert_of` the original id, original block), a journaled update or delete fails with an
// [IrreversibleError]. The journal is left untouched, call [Journal.Discard] once the compensations are emitted.
func (j *Journal) Compensations(lastValidBlock uint64, cursor string) ([]Event, error) {
	var out []Event
	for i := len(j.blocks) - 1; i >= 0 && j.blocks[i].block.Number > lastValidBlock; i-- {
		events := j.blocks[i].events
		for k := len(events) - 1; k >= 0; k-- {
			event := events[k]
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
	}
	return out, nil
}

// JournalPath derives the journal file from the sink state (cursor) file.
func JournalPath(stateFile string) string {
	if stateFile == "" {
		return ""
	}
	return stateFile + ".journal"
}

// Save writes the journal, tagged with cursor, as a protobuf [pbcdc.JournalState] using a temp file and a
// rename. It does nothing when path is empty.
//
// Save it before writing the cursor file: a crash in between leaves a journal ahead of the cursor, which is
// harmless since replayed events are deduplicated by [Journal.Add], while the reverse would lose events a
// later undo needs to compensate.
func (j *Journal) Save(path string, cursor string) error {
	if path == "" {
		return nil
	}

	state := &pbcdc.JournalState{Cursor: cursor}
	for _, page := range j.blocks {
		pbBlock := &pbcdc.JournalBlock{Number: page.block.Number, Id: page.block.ID}
		for i := range page.events {
			pbEvent, err := toProtoEvent(&page.events[i])
			if err != nil {
				return err
			}
			pbBlock.Events = append(pbBlock.Events, pbEvent)
		}
		state.Blocks = append(state.Blocks, pbBlock)
	}

	data, err := proto.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode journal: %w", err)
	}

	return sink.WriteFileAtomic(path, data)
}

// LoadJournal reads a journal saved by [Journal.Save] and the cursor it was saved at. A missing file, or an
// empty path, gives an empty journal.
func LoadJournal(path string) (*Journal, string, error) {
	journal := NewJournal()
	if path == "" {
		return journal, "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return journal, "", nil
		}
		return nil, "", err
	}

	state := &pbcdc.JournalState{}
	if err := proto.Unmarshal(data, state); err != nil {
		return nil, "", fmt.Errorf("decode journal %q: %w", path, err)
	}

	for _, pbBlock := range state.Blocks {
		for _, pbEvent := range pbBlock.Events {
			event, err := fromProtoEvent(pbBlock, pbEvent)
			if err != nil {
				return nil, "", fmt.Errorf("journal %q: %w", path, err)
			}
			journal.Add(event)
		}
	}

	return journal, state.Cursor, nil
}
