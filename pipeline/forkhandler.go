package pipeline

import (
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type UndoHandler func(clock *pbsubstreams.Clock, moduleOutputs []*pbsubstreams.ModuleOutput)

type ForkHandler struct {
	reversibleOutputs map[uint64][]*pbsubstreams.ModuleOutput
	undoHandlers      []UndoHandler
}

func NewForkHandler() *ForkHandler {
	return &ForkHandler{
		reversibleOutputs: make(map[uint64][]*pbsubstreams.ModuleOutput),
		undoHandlers:      []UndoHandler{},
	}
}

func (f *ForkHandler) registerUndoHandler(handler UndoHandler) {
	f.undoHandlers = append(f.undoHandlers, handler)
}

func (f *ForkHandler) handleUndo(
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	respFunc func(resp *pbsubstreams.Response) error,
) error {
	if moduleOutputs, found := f.reversibleOutputs[clock.Number]; found {
		if err := returnModuleDataOutputs(clock, bstream.StepUndo, cursor, moduleOutputs, respFunc); err != nil {
			return fmt.Errorf("calling return func when reverting outputs: %w", err)
		}

		for _, h := range f.undoHandlers {
			h(clock, moduleOutputs)
		}
	}
	return nil
}

func (f *ForkHandler) removeReversibleOutput(blockNumber uint64) {
	delete(f.reversibleOutputs, blockNumber)
}

func (f *ForkHandler) addReversibleOutput(moduleOutput *pbsubstreams.ModuleOutput, blockNum uint64) {
	f.reversibleOutputs[blockNum] = append(f.reversibleOutputs[blockNum], moduleOutput)
}

type DeltaGetter interface {
	GetDeltas() []*pbsubstreams.StoreDelta
}
