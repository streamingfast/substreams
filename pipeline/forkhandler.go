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

func (f *ForkHandler) revertOutputs(
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
			reverseDeltas(storeMap, moduleOutput)
		}
	}
	return nil
}

func (f *ForkHandler) handleIrreversibility(blockNumber uint64) {
	delete(f.reversibleOutputs, blockNumber)
}

func (f *ForkHandler) addModuleOutput(moduleOutput *pbsubstreams.ModuleOutput, blockNum uint64) {
	f.reversibleOutputs[blockNum] = append(f.reversibleOutputs[blockNum], moduleOutput)
}

func reverseDeltas(storeMap map[string]*state.Store, moduleOutput *pbsubstreams.ModuleOutput) {
	if store, allRight := storeMap[moduleOutput.Name]; allRight {
		store.ApplyDeltaReverse(moduleOutput.GetStoreDeltas().GetDeltas())
	}
}
