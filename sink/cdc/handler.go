package cdc

import (
	"context"
	"errors"
	"fmt"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

// BlockInput is what an [EventSource] gets to turn a block's module output into events.
type BlockInput struct {
	// Module is the name of the output module.
	Module string
	// Output is the module's output for the block, it can be nil when the module produced nothing.
	Output *anypb.Any
	Clock  *pbsubstreams.Clock
	// FinalBlockHeight is the highest block known to be final when this block was produced.
	FinalBlockHeight uint64
	// Final is true when the block itself is final.
	Final bool
}

// EventSource turns a block's module output into change events. It fills ID (optional), Op, Key, After,
// Partial, Source.Table and Source.Ordinal, the [Handler] fills the rest of Source.
type EventSource interface {
	Events(ctx context.Context, in *BlockInput) ([]Event, error)
}

// EventSourceFunc adapts a function to an [EventSource].
type EventSourceFunc func(ctx context.Context, in *BlockInput) ([]Event, error)

func (f EventSourceFunc) Events(ctx context.Context, in *BlockInput) ([]Event, error) {
	return f(ctx, in)
}

// HandlerOptions configures a [Handler].
type HandlerOptions struct {
	Logger *zap.Logger
	// ReversibleBlocks tells the stream can deliver undo signals, which is the case unless final blocks only
	// are requested or an undo buffer is used, see [ReversibleBlocksFromConfig]. When set, updates and deletes
	// in a non-final block are refused before anything is emitted, since they could not be reverted.
	ReversibleBlocks bool
	// StateFile is the sink cursor file. When set, the handler writes the cursor there, and the journal next
	// to it (see [JournalPath]), after each block.
	StateFile string
}

// ReversibleBlocksFromConfig reads from the sinker configuration whether the stream can deliver undo signals.
func ReversibleBlocksFromConfig(config *sink.SinkerConfig) bool {
	return !config.FinalBlocksOnly && config.UndoBufferSize <= 0
}

// Handler is a [sink.SinkerHandler] that turns each block into change events, delivers them to an
// [Emitter], and answers undo signals with compensation events from a [Journal].
type Handler struct {
	source  EventSource
	emitter Emitter
	journal *Journal
	opts    HandlerOptions
}

var (
	_ sink.SinkerHandler           = (*Handler)(nil)
	_ sink.SinkerCompletionHandler = (*Handler)(nil)
)

// NewHandler creates the handler. A nil journal gets a fresh one when opts.ReversibleBlocks is set, and
// stays disabled otherwise (no journaling, an undo signal is then an error). To resume after a restart,
// pass the journal from [LoadJournal].
func NewHandler(source EventSource, emitter Emitter, journal *Journal, opts HandlerOptions) *Handler {
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	if journal == nil && opts.ReversibleBlocks {
		journal = NewJournal()
	}

	return &Handler{source: source, emitter: emitter, journal: journal, opts: opts}
}

func (h *Handler) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	clock := data.Clock
	final := clock.Number <= data.FinalBlockHeight
	module := ""
	var output *anypb.Any
	if data.Output != nil {
		module = data.Output.Name
		output = data.Output.MapOutput
	}

	events, err := h.source.Events(ctx, &BlockInput{
		Module:           module,
		Output:           output,
		Clock:            clock,
		FinalBlockHeight: data.FinalBlockHeight,
		Final:            final,
	})
	if err != nil {
		return fmt.Errorf("event source for block #%d: %w", clock.Number, err)
	}

	block := Block{Number: clock.Number, ID: clock.Id, Timestamp: clock.Timestamp.AsTime().UTC()}
	tableCounts := map[string]int{}
	for i := range events {
		event := &events[i]
		if !event.Op.Valid() {
			return fmt.Errorf("event source produced an invalid op %q for table %q at block #%d", event.Op, event.Source.Table, clock.Number)
		}

		// Checked before anything is emitted, so a refused block leaves no partial output.
		if h.opts.ReversibleBlocks && !final && event.Op != OpCreate {
			return fmt.Errorf(
				"module %q table %q produced a %s event at block #%d which is not final (final block height is #%d) and could be reverted by a reorg, "+
					"but updates and deletes can't be reverted since no before-image is available. Run with %s (final blocks only) or %s N (pseudo-final blocks) to allow them",
				module, event.Source.Table, opName(event.Op), clock.Number, data.FinalBlockHeight, "--"+sink.FlagFinalBlocksOnly, "--"+sink.FlagUndoBufferSize,
			)
		}

		if event.Source.Module == "" {
			event.Source.Module = module
		}
		event.Source.Block = block
		event.Source.Final = final
		event.Source.Cursor = data.Cursor
		if event.ID == "" {
			event.ID = fmt.Sprintf("%s:%s:%d", block.ID, event.Source.Table, tableCounts[event.Source.Table])
		}
		tableCounts[event.Source.Table]++
	}

	if len(events) > 0 {
		if err := h.emitter.EmitBatch(ctx, &Batch{Block: block, Cursor: data.Cursor, Events: events}); err != nil {
			return fmt.Errorf("emit block #%d: %w", clock.Number, err)
		}
	}

	if h.journal != nil {
		if !final {
			h.journal.Add(events...)
		}
		h.journal.Prune(data.FinalBlockHeight)
	}

	return h.checkpoint(cursor, data.Cursor)
}

func (h *Handler) HandleBlockUndoSignal(ctx context.Context, undo *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	if h.journal == nil {
		return errors.New("received an undo signal but the handler has no journal: it was not told the stream can deliver undo signals")
	}

	lastValid := undo.LastValidBlock.Number
	compensations, err := h.journal.Compensations(lastValid, undo.LastValidCursor)
	if err != nil {
		return err
	}

	if len(compensations) > 0 {
		batch := &Batch{
			Block:  Block{Number: lastValid, ID: undo.LastValidBlock.Id},
			Cursor: undo.LastValidCursor,
			Events: compensations,
		}
		if err := h.emitter.EmitCompensation(ctx, batch); err != nil {
			return fmt.Errorf("emit compensation after undo to block #%d: %w", lastValid, err)
		}
	}

	h.journal.Discard(lastValid)
	return h.checkpoint(cursor, undo.LastValidCursor)
}

// HandleBlockRangeCompletion flushes the emitter once the requested range is fully processed.
func (h *Handler) HandleBlockRangeCompletion(ctx context.Context, cursor *sink.Cursor) error {
	if err := h.emitter.Flush(ctx); err != nil {
		return fmt.Errorf("flush emitter: %w", err)
	}
	return nil
}

// checkpoint persists the journal and then the cursor, in that order, see [Journal.Save].
func (h *Handler) checkpoint(cursor *sink.Cursor, cursorStr string) error {
	if h.opts.StateFile == "" {
		return nil
	}

	if h.journal != nil {
		if err := h.journal.Save(JournalPath(h.opts.StateFile), cursorStr); err != nil {
			return fmt.Errorf("save journal: %w", err)
		}
	}
	if err := sink.WriteCursor(h.opts.StateFile, cursor); err != nil {
		return fmt.Errorf("write cursor: %w", err)
	}
	return nil
}
