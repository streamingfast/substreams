package config

import (
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/client"
)

type RuntimeConfig struct {
	StoreSnapshotsSaveInterval uint64
	ExecOutputSaveInterval     uint64

	SubrequestsSplitSize int // in multiple of the SaveIntervals above
	ParallelSubrequests  int // how many sub-jobs to launch for a given user

	// derives substores `states/`, for `store` modules snapshots (full and partial)
	// and `outputs/` for execution output of both `map` and `store` module kinds
	BaseObjectStore dstore.Store

	SubstreamsClientFactory client.Factory
}
