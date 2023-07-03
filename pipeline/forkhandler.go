package pipeline

import (
	"github.com/streamingfast/bstream"
	"sync"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type UndoHandler func(clock *pbsubstreams.Clock, moduleOutputs []*pbssinternal.ModuleOutput)

// TODO(abourget): The scope of this object and the Engine
//
//	are not pretty similar, to keep track of certain pieces
//	of info that are reversible, and handle the back and forth
//	between undos and redos.
//	Perhaps what we could have here, is have those undo handlers
//	live on the Pipeline (where it makes sense)
//	and have some nested structs handle
type ForkHandler struct {
	reversibleOutputs map[string][]*pbssinternal.ModuleOutput
	undoHandlers      []UndoHandler

	mu sync.RWMutex
}

func NewForkHandler() *ForkHandler {
	return &ForkHandler{
		reversibleOutputs: make(map[string][]*pbssinternal.ModuleOutput),
		undoHandlers:      []UndoHandler{},
	}
}

func (f *ForkHandler) registerUndoHandler(handler UndoHandler) {
	f.undoHandlers = append(f.undoHandlers, handler)
}

func (f *ForkHandler) handleUndo(
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if moduleOutputs, found := f.reversibleOutputs[clock.Id]; found {
		for _, h := range f.undoHandlers {
			h(clock, moduleOutputs)
		}
	}
	return nil
}

func (f *ForkHandler) removeReversibleOutput(blockID string) {
	f.mu.Lock()
	delete(f.reversibleOutputs, blockID)
	f.mu.Unlock()
}

func (f *ForkHandler) addReversibleOutput(moduleOutput *pbssinternal.ModuleOutput, blockID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reversibleOutputs[blockID] = append(f.reversibleOutputs[blockID], moduleOutput)
}
