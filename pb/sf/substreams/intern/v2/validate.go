package pbssinternal

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (r *ProcessRangeRequest) Validate() error {
	if r.StartBlockNum >= r.StopBlockNum {
		return fmt.Errorf("stop block %d should be higher than start block %d", r.StopBlockNum, r.StartBlockNum)
	}

	if r.Modules == nil {
		return fmt.Errorf("no modules found in request")
	}

	if r.OutputModule == "" {
		return fmt.Errorf("no output module defined in request")
	}

	seenStores := map[string]bool{}
	outputModuleFound := false
	for _, mod := range r.Modules.Modules {
		if _, ok := mod.Kind.(*pbsubstreams.Module_KindStore_); ok {
			seenStores[mod.Name] = true
		}
		if mod.Name == r.OutputModule { // internal request can have store or module output
			outputModuleFound = true
		}
	}
	if !outputModuleFound {
		return fmt.Errorf("output module %q not found in modules", r.OutputModule)
	}

	return nil
}
