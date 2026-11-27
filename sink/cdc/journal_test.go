package cdc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/sink"
)

func ids(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}

func TestJournal_AddDedupAndPrune(t *testing.T) {
	j := NewJournal()
	j.Add(newEvent("a", OpCreate, 10, "t"), newEvent("b", OpCreate, 11, "t"))
	j.Add(newEvent("a", OpCreate, 10, "t"))
	assert.Equal(t, []string{"a", "b"}, ids(j.Events()))

	j.Prune(10)
	assert.Equal(t, []string{"b"}, ids(j.Events()))
	j.Prune(11)
	assert.Equal(t, 0, j.Len())
}

func TestJournal_CompensationsReverseOrder(t *testing.T) {
	j := NewJournal()
	j.Add(
		newEvent("a", OpCreate, 10, "t"),
		newEvent("b", OpCreate, 11, "t"),
		newEvent("c", OpCreate, 11, "u"),
		newEvent("d", OpCreate, 12, "t"),
	)

	comps, err := j.Compensations(10, "undo-cursor")
	require.NoError(t, err)
	assert.Equal(t, []string{"d:revert", "c:revert", "b:revert"}, ids(comps))

	first := comps[0]
	assert.Equal(t, OpDelete, first.Op)
	assert.Equal(t, "d", first.RevertOf)
	assert.Nil(t, first.After)
	assert.Equal(t, map[string]string{"id": "d"}, first.Key)
	assert.EqualValues(t, 12, first.Source.Block.Number, "compensation keeps the original block")
	assert.Equal(t, "undo-cursor", first.Source.Cursor)
	assert.False(t, first.Source.Final)

	assert.Equal(t, 4, j.Len(), "journal untouched until Discard")
	j.Discard(10)
	assert.Equal(t, []string{"a"}, ids(j.Events()))
}

func TestJournal_CompensationsIrreversible(t *testing.T) {
	for _, op := range []Op{OpUpdate, OpDelete} {
		j := NewJournal()
		j.Add(newEvent("a", OpCreate, 10, "t"), newEvent("b", op, 11, "tbl"))

		_, err := j.Compensations(10, "c")
		var irr *IrreversibleError
		require.ErrorAs(t, err, &irr)
		assert.Equal(t, "b", irr.Event.ID)
		assert.Contains(t, err.Error(), "--"+sink.FlagFinalBlocksOnly)
		assert.Contains(t, err.Error(), "--"+sink.FlagUndoBufferSize)
		assert.Contains(t, err.Error(), `table "tbl"`)

		// Same events but the update/delete is at or below the last valid block: fine.
		comps, err := j.Compensations(11, "c")
		require.NoError(t, err)
		assert.Empty(t, comps)
	}
}

func TestJournal_SaveLoadRoundTrip(t *testing.T) {
	path := JournalPath(filepath.Join(t.TempDir(), "state.cursor"))
	j := NewJournal()
	e := newEvent("a", OpCreate, 10, "t")
	e.After = map[string]any{"amount": float64(12)}
	j.Add(e, newEvent("b", OpCreate, 11, "t"))
	require.NoError(t, j.Save(path, "cur-11"))

	loaded, cursor, err := LoadJournal(path)
	require.NoError(t, err)
	assert.Equal(t, "cur-11", cursor)
	assert.Equal(t, []string{"a", "b"}, ids(loaded.Events()))
	assert.Equal(t, e.Source.Block, loaded.Events()[0].Source.Block)
}

func TestJournal_LoadMissing(t *testing.T) {
	j, cursor, err := LoadJournal(filepath.Join(t.TempDir(), "nope.journal"))
	require.NoError(t, err)
	assert.Equal(t, 0, j.Len())
	assert.Empty(t, cursor)
	assert.Empty(t, JournalPath(""))
}

func TestJournal_RestartOnFork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "j")
	j := NewJournal()
	j.Add(newEvent("a", OpCreate, 10, "t"), newEvent("b", OpCreate, 11, "t"))
	require.NoError(t, j.Save(path, "cur"))

	// Process restarts, then the stream tells us block 11 was orphaned.
	restarted, _, err := LoadJournal(path)
	require.NoError(t, err)
	comps, err := restarted.Compensations(10, "u")
	require.NoError(t, err)
	assert.Equal(t, []string{"b:revert"}, ids(comps))
}

func TestJournal_PagedByBlock(t *testing.T) {
	j := NewJournal()
	c := newEvent("c", OpCreate, 11, "t")
	c.Source.Block.ID = "bb"
	j.Add(newEvent("a", OpCreate, 10, "t"), newEvent("b", OpCreate, 11, "t"), c)
	assert.Len(t, j.blocks, 2)
	assert.Equal(t, 3, j.Len())

	j.Discard(10)
	assert.Len(t, j.blocks, 1)
	assert.Equal(t, 1, j.Len())

	// A discarded block's ids can be journaled again (the fork replays the same ids only on the same block).
	j.Add(newEvent("b", OpCreate, 11, "t"))
	assert.Equal(t, 2, j.Len())
}
