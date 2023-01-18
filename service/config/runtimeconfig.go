package config

import (
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/orchestrator/work"
)

type RuntimeConfig struct {
	CacheSaveInterval uint64

	SubrequestsSplitSize uint64 // in multiple of the SaveIntervals above
	ParallelSubrequests  uint64 // how many sub-jobs to launch for a given user
	// derives substores `states/`, for `store` modules snapshots (full and partial)
	// and `outputs/` for execution output of both `map` and `store` module kinds
	BaseObjectStore  dstore.Store
	WorkerFactory    work.WorkerFactory
	WithRequestStats bool
}

func NewRuntimeConfig(
	cacheSaveInterval uint64,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64,
	baseObjectStore dstore.Store,
	workerFactory work.WorkerFactory,
) RuntimeConfig {
	return RuntimeConfig{
		CacheSaveInterval:    cacheSaveInterval,
		SubrequestsSplitSize: subrequestsSplitSize,
		ParallelSubrequests:  parallelSubrequests,
		BaseObjectStore:      baseObjectStore,
		WorkerFactory:        workerFactory,
	}
}
