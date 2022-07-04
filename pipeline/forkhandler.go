package pipeline

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"strconv"
)

type ForkHandler struct {
	reversibleOutputs map[string][]*pbsubstreams.ModuleOutput
}

func (f *ForkHandler) revertOutputs(modules []*pbsubstreams.Module) error {
	for output := range f.reversibleOutputs {
		_ = output
		return nil
	}
}

func (f *ForkHandler) handleIrreversibility(blockNumber uint64) {
	delete(f.reversibleOutputs, strconv.FormatUint(blockNumber, 10))
}
