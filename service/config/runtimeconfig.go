package config

import (
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/orchestrator/work"
)

type RuntimeConfig struct {
	CacheSaveInterval uint64

	MaxWasmFuel          uint64 // if not 0, enable fuel consumption monitoring to stop runaway wasm module processing forever
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
	MaxWasmFuel uint64,
	baseObjectStore dstore.Store,
	workerFactory work.WorkerFactory,
) RuntimeConfig {
	return RuntimeConfig{
		CacheSaveInterval:    cacheSaveInterval,
		SubrequestsSplitSize: subrequestsSplitSize,
		ParallelSubrequests:  parallelSubrequests,
		MaxWasmFuel:          MaxWasmFuel,
		BaseObjectStore:      baseObjectStore,
		WorkerFactory:        workerFactory,
	}
}
