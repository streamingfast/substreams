package pipeline

import (
	"fmt"
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
)

type ForkHandler struct {
	reversibleOutputs map[uint64][]*pbsubstreams.ModuleOutput
}

func NewForkHandle() *ForkHandler {
	return &ForkHandler{
		reversibleOutputs: make(map[uint64][]*pbsubstreams.ModuleOutput),
	}
}

func (f *ForkHandler) handleUndo(
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	moduleOutputCache *outputs.ModulesOutputCache,
	storeMap map[string]*state.Store,
	respFunc func(resp *pbsubstreams.Response) error,
) error {
	if moduleOutputs, found := f.reversibleOutputs[clock.Number]; found {
		if err := returnModuleDataOutputs(clock, bstream.StepUndo, cursor, moduleOutputs, respFunc); err != nil {
			return fmt.Errorf("calling return func when reverting outputs: %w", err)
		}
		for _, moduleOutput := range moduleOutputs {
			if outputCache, ok := moduleOutputCache.OutputCaches[moduleOutput.Name]; ok {
				outputCache.Delete(clock.Id)
			}
			reverseDeltas(storeMap, moduleOutput.Name, moduleOutput.GetStoreDeltas())
		}
	}
	return nil
}

func (f *ForkHandler) handleIrreversible(blockNumber uint64) {
	delete(f.reversibleOutputs, blockNumber)
}

func (f *ForkHandler) addModuleOutput(moduleOutput *pbsubstreams.ModuleOutput, blockNum uint64) {
	f.reversibleOutputs[blockNum] = append(f.reversibleOutputs[blockNum], moduleOutput)
}

type DeltaGetter interface {
	GetDeltas() []*pbsubstreams.StoreDelta
}

func reverseDeltas(storeMap map[string]*state.Store, name string, deltaGetter DeltaGetter) {
	if store, found := storeMap[name]; found {
		store.ApplyDeltaReverse(deltaGetter.GetDeltas())
	}
}
