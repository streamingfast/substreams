package squash

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// Range is one store segment to merge, [StartBlock, ExclusiveEndBlock).
type Range struct {
	StartBlock        uint64
	ExclusiveEndBlock uint64
}

// Request is one store's squash run. Ranges are contiguous segments in order.
// The standalone squasher implements Client and maps Ranges onto
// SquashRequest.ranges (proto field 11). This module does not import that proto.
type Request struct {
	ModuleName           string
	ModuleHash           string
	ModuleInitialBlock   uint64
	UpdatePolicy         pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType            string
	StateStore           string
	StateStoreDefaultTag string
	Ranges               []Range
	StoreSizeLimit       uint64
}

type Result struct {
	ExclusiveEndBlock uint64
	StoreSizeBytes    uint64
	LoadedExisting    bool
}

type Client interface {
	Squash(ctx context.Context, req Request) (*Result, error)
}
