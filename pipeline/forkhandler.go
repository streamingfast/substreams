package pipeline

import (
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type UndoHandler func(clock *pbsubstreams.Clock, moduleOutputs []*pbsubstreams.ModuleOutput)

// TODO(abourget): The scope of this object and the Engine
//
//	are not pretty similar, to keep track of certain pieces
//	of info that are reversible, and handle the back and forth
//	between undos and redos.
//	Perhaps what we could have here, is have those undo handlers
//	live on the Pipeline (where it makes sense)
//	and have some nested structs handle
type ForkHandler struct {
	reversibleOutputs map[string][]*pbsubstreams.ModuleOutput
	undoHandlers      []UndoHandler
}

func NewForkHandler() *ForkHandler {
	return &ForkHandler{
		reversibleOutputs: make(map[string][]*pbsubstreams.ModuleOutput),
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
	sendOutputs bool,
	outputableModules []*pbsubstreams.Module,
) error {
	if moduleOutputs, found := f.reversibleOutputs[clock.Id]; found {
		if sendOutputs {
			var toSend []*pbsubstreams.ModuleOutput
			for _, out := range moduleOutputs {
				for _, outputable := range outputableModules {
					if outputable.Name == out.Name {
						toSend = append(toSend, out)
						break
					}
				}
			}
			if err := returnModuleDataOutputs(clock, bstream.StepUndo, cursor, toSend, respFunc); err != nil {
				return fmt.Errorf("calling return func when reverting outputs: %w", err)
			}
		}

		for _, h := range f.undoHandlers {
			h(clock, moduleOutputs)
		}
	}
	return nil
}

func (f *ForkHandler) removeReversibleOutput(blockID string) {
	delete(f.reversibleOutputs, blockID)
}

func (f *ForkHandler) addReversibleOutput(moduleOutput *pbsubstreams.ModuleOutput, blockID string) {
	f.reversibleOutputs[blockID] = append(f.reversibleOutputs[blockID], moduleOutput)
}
