package pipeline

import (
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
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
	storeMap store.Map,
	respFunc func(resp *pbsubstreams.Response) error,
) error {
	if moduleOutputs, found := f.reversibleOutputs[clock.Number]; found {
		if err := returnModuleDataOutputs(clock, bstream.StepUndo, cursor, moduleOutputs, respFunc); err != nil {
			return fmt.Errorf("calling return func when reverting outputs: %w", err)
		}
		for _, moduleOutput := range moduleOutputs {
			if s, found := storeMap.Get(moduleOutput.Name); found {
				if deltaStore, ok := s.(store.DeltaAccessor); ok {
					deltaStore.ApplyDeltasReverse(moduleOutput.GetStoreDeltas().GetDeltas())
				}
			}
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
