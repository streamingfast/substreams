package pbssinternal

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (r *ProcessRangeRequest) Validate() error {
	switch {
	case r.StopBlockNum != 0:
		return fmt.Errorf("invalid protocol: update your tier1")
	case r.Modules == nil:
		return fmt.Errorf("no modules found in request")
	case r.OutputModule == "":
		return fmt.Errorf("no output module defined in request")
	case r.MeteringConfig == "":
		return fmt.Errorf("metering config is required in request")
	case r.BlockType == "":
		return fmt.Errorf("block type is required in request")
	case r.StateStore == "":
		return fmt.Errorf("state store is required in request")
	case r.MergedBlocksStore == "":
		return fmt.Errorf("merged blocks store is required in request")
	case r.SegmentSize == 0:
		return fmt.Errorf("a non-zero state bundle size is required in request")
	case ((r.SegmentNumber+1)*r.SegmentSize - 1) < r.FirstStreamableBlock:
		return fmt.Errorf("segment is completely below the first streamable block")
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
