package cdc

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
)

type recordingEmitter struct {
	batches       []*Batch
	compensations []*Batch
	flushed       bool
}

func (r *recordingEmitter) EmitBatch(_ context.Context, b *Batch) error {
	r.batches = append(r.batches, b)
	return nil
}
func (r *recordingEmitter) EmitCompensation(_ context.Context, b *Batch) error {
	r.compensations = append(r.compensations, b)
	return nil
}
func (r *recordingEmitter) Flush(context.Context) error { r.flushed = true; return nil }
func (r *recordingEmitter) Close() error                { return nil }

func blockData(number uint64, final uint64) *pbsubstreamsrpc.BlockScopedData {
	return &pbsubstreamsrpc.BlockScopedData{
		Output:           &pbsubstreamsrpc.MapModuleOutput{Name: "db_out"},
		Clock:            &pbsubstreams.Clock{Id: "blk" + string(rune('0'+number%10)), Number: number, Timestamp: timestamppb.Now()},
		Cursor:           "cursor-" + string(rune('0'+number%10)),
		FinalBlockHeight: final,
	}
}

func sourceOf(ops ...Op) EventSource {
	return EventSourceFunc(func(_ context.Context, in *BlockInput) ([]Event, error) {
		var out []Event
		for i, op := range ops {
			out = append(out, Event{Op: op, Key: map[string]string{"k": "v"}, Source: Source{Table: "t", Ordinal: uint64(i)}})
		}
		return out, nil
	})
}

func testCursor(num uint64, id string) *sink.Cursor {
	ref := bstream.NewBlockRef(id, num)
	return &sink.Cursor{Cursor: &bstream.Cursor{Step: bstream.StepNew, Block: ref, LIB: ref, HeadBlock: ref}}
}

func TestHandler_FillsSourceAndIDs(t *testing.T) {
	em := &recordingEmitter{}
	h := NewHandler(sourceOf(OpCreate, OpCreate), em, nil, HandlerOptions{ReversibleBlocks: true})

	require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(5, 3), nil, nil))
	require.Len(t, em.batches, 1)
	events := em.batches[0].Events
	assert.Equal(t, "blk5:t:0", events[0].ID)
	assert.Equal(t, "blk5:t:1", events[1].ID)
	assert.Equal(t, "db_out", events[0].Source.Module)
	assert.EqualValues(t, 5, events[0].Source.Block.Number)
	assert.False(t, events[0].Source.Final)
	assert.Equal(t, "cursor-5", events[0].Source.Cursor)
	assert.Equal(t, 2, h.journal.Len())
}

func TestHandler_FinalNotJournaledAndPrunes(t *testing.T) {
	em := &recordingEmitter{}
	h := NewHandler(sourceOf(OpCreate), em, nil, HandlerOptions{ReversibleBlocks: true})

	require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(5, 3), nil, nil))
	assert.Equal(t, 1, h.journal.Len())
	require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(6, 5), nil, nil))
	assert.Equal(t, 1, h.journal.Len(), "block 5 pruned at final height 5, block 6 kept")
	require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(7, 7), nil, nil))
	assert.Equal(t, 0, h.journal.Len())
	assert.True(t, em.batches[2].Events[0].Source.Final)
}

func TestHandler_FailFastOnUpdateDelete(t *testing.T) {
	for _, op := range []Op{OpUpdate, OpDelete} {
		em := &recordingEmitter{}
		h := NewHandler(sourceOf(OpCreate, op), em, nil, HandlerOptions{ReversibleBlocks: true})

		err := h.HandleBlockScopedData(t.Context(), blockData(5, 3), nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `module "db_out" table "t"`)
		assert.Contains(t, err.Error(), FlagFinalBlocksOnly)
		assert.Contains(t, err.Error(), FlagUndoBufferSize)
		assert.Empty(t, em.batches, "nothing emitted for a refused block")
		assert.Equal(t, 0, h.journal.Len())

		// Final block: allowed.
		require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(5, 5), nil, nil))

		// Stream cannot undo: allowed even when not final.
		em2 := &recordingEmitter{}
		h2 := NewHandler(sourceOf(op), em2, nil, HandlerOptions{})
		require.NoError(t, h2.HandleBlockScopedData(t.Context(), blockData(5, 3), nil, nil))
		assert.Len(t, em2.batches, 1)
		assert.Nil(t, h2.journal)
	}
}

func TestHandler_Undo(t *testing.T) {
	em := &recordingEmitter{}
	h := NewHandler(sourceOf(OpCreate), em, nil, HandlerOptions{ReversibleBlocks: true})
	for n := uint64(5); n <= 7; n++ {
		require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(n, 4), nil, nil))
	}

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Id: "blk5", Number: 5}, LastValidCursor: "cursor-5"}
	require.NoError(t, h.HandleBlockUndoSignal(t.Context(), undo, nil))

	require.Len(t, em.compensations, 1)
	comps := em.compensations[0].Events
	assert.Equal(t, []string{"blk7:t:0:revert", "blk6:t:0:revert"}, ids(comps))
	assert.Equal(t, "cursor-5", em.compensations[0].Cursor)
	assert.Equal(t, 1, h.journal.Len())
}

func TestHandler_UndoIrreversibleKeepsJournal(t *testing.T) {
	em := &recordingEmitter{}
	h := NewHandler(sourceOf(), em, nil, HandlerOptions{ReversibleBlocks: true})
	h.journal.Add(newEvent("x", OpUpdate, 6, "t"))

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Id: "blk5", Number: 5}}
	err := h.HandleBlockUndoSignal(t.Context(), undo, nil)
	var irr *IrreversibleError
	require.ErrorAs(t, err, &irr)
	assert.Empty(t, em.compensations)
	assert.Equal(t, 1, h.journal.Len())
}

func TestHandler_UndoWithoutJournal(t *testing.T) {
	h := NewHandler(sourceOf(), &recordingEmitter{}, nil, HandlerOptions{})
	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 5}}
	require.Error(t, h.HandleBlockUndoSignal(t.Context(), undo, nil))
}

func TestHandler_PersistenceAndRestartOnFork(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.cursor")

	em := &recordingEmitter{}
	h := NewHandler(sourceOf(OpCreate), em, nil, HandlerOptions{ReversibleBlocks: true, StateFile: stateFile})
	require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(5, 4), nil, testCursor(5, "blk5")))
	require.NoError(t, h.HandleBlockScopedData(t.Context(), blockData(6, 4), nil, testCursor(6, "blk6")))

	cursor, err := sink.ReadCursor(stateFile)
	require.NoError(t, err)
	assert.EqualValues(t, 6, cursor.Block().Num())

	// Restart: new handler, journal loaded from disk, stream then reports a fork back to block 5.
	journal, savedCursor, err := LoadJournal(JournalPath(stateFile))
	require.NoError(t, err)
	assert.Equal(t, "cursor-6", savedCursor)

	em2 := &recordingEmitter{}
	h2 := NewHandler(sourceOf(OpCreate), em2, journal, HandlerOptions{ReversibleBlocks: true, StateFile: stateFile})
	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Id: "blk5", Number: 5}, LastValidCursor: "cursor-5"}
	require.NoError(t, h2.HandleBlockUndoSignal(t.Context(), undo, testCursor(5, "blk5")))
	require.Len(t, em2.compensations, 1)
	assert.Equal(t, []string{"blk6:t:0:revert"}, ids(em2.compensations[0].Events))

	reloaded, cur, err := LoadJournal(JournalPath(stateFile))
	require.NoError(t, err)
	assert.Equal(t, "cursor-5", cur)
	assert.Equal(t, []string{"blk5:t:0"}, ids(reloaded.Events()))
	assert.FileExists(t, stateFile)
}

func TestHandler_Completion(t *testing.T) {
	em := &recordingEmitter{}
	h := NewHandler(sourceOf(), em, nil, HandlerOptions{})
	require.NoError(t, h.HandleBlockRangeCompletion(t.Context(), nil))
	assert.True(t, em.flushed)
}

func TestReversibleBlocksFromConfig(t *testing.T) {
	assert.True(t, ReversibleBlocksFromConfig(&sink.SinkerConfig{}))
	assert.False(t, ReversibleBlocksFromConfig(&sink.SinkerConfig{FinalBlocksOnly: true}))
	assert.False(t, ReversibleBlocksFromConfig(&sink.SinkerConfig{UndoBufferSize: 12}))
}
