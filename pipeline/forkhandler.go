package pipeline

import (
	"slices"
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

// joinReversibleOutputs will take the reversible outputs of multiple partial blocks and squash them into one "undoable" block with the target ID.
// ex: target= "1f", previousClocks = "1a","1b","1c","1d","1e" -> the undo material will be joined to be undone in proper order (1f, 1e, 1d, 1c, 1b, 1a)
func (f *ForkHandler) joinReversibleOutputs(target *pbsubstreams.Clock, previousClocks []*pbsubstreams.Clock) {
	f.mu.Lock()
	defer f.mu.Unlock()

	reversibleOutputs := f.reversibleOutputs[target.Id]
	for _, v := range slices.Backward(previousClocks) {
		if clock := v; clock != nil {
			if outputs, found := f.reversibleOutputs[clock.Id]; found {
				reversibleOutputs = append(reversibleOutputs, outputs...)
				delete(f.reversibleOutputs, clock.Id)
			}
		}
	}

	f.reversibleOutputs[target.Id] = reversibleOutputs

}

func (f *ForkHandler) handleUndo(
	clock *pbsubstreams.Clock,
) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if moduleOutputs, found := f.reversibleOutputs[clock.Id]; found {
		for _, h := range f.undoHandlers {
			h(clock, moduleOutputs)
		}
		delete(f.reversibleOutputs, clock.Id)
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
