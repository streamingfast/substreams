package pipeline

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ForkHandler struct {
	reversibleOutputs map[uint64][]*pbsubstreams.ModuleOutput
}

func (f *ForkHandler) revertOutputs(blockNum uint64) error {
	if moduleOutputs, found := f.reversibleOutputs[blockNum]; found {
		for moduleOutput := range moduleOutputs {
			_ = moduleOutput
			// todo: need to revert the apply deltas
		}
	}

	return nil
}

func (f *ForkHandler) handleIrreversibility(blockNumber uint64) {
	delete(f.reversibleOutputs, blockNumber)
}
