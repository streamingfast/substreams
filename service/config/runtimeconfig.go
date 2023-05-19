package config

import (
	"github.com/streamingfast/dstore"

	"github.com/streamingfast/substreams/orchestrator/work"
)

// RuntimeConfig is a global configuration for the service.
// It is passed down and should not be modified unless cloned.
type RuntimeConfig struct {
	CacheSaveInterval uint64

	MaxWasmFuel          uint64 // if not 0, enable fuel consumption monitoring to stop runaway wasm module processing forever
	SubrequestsSplitSize uint64 // in multiple of the SaveIntervals above
	MaxJobsAhead         uint64 // limit execution of depencency jobs so they don't go too far ahead of the modules that depend on them (ex: module X is 2 million blocks ahead of module Y that depends on it, we don't want to schedule more module X jobs until Y caught up a little bit)
	ParallelSubrequests  uint64 // how many sub-jobs to launch for a given user
	// derives substores `states/`, for `store` modules snapshots (full and partial)
	// and `outputs/` for execution output of both `map` and `store` module kinds
	BaseObjectStore dstore.Store
	WorkerFactory   work.WorkerFactory

	WithRequestStats       bool
	ModuleExecutionTracing bool
}

func NewRuntimeConfig(
	cacheSaveInterval uint64,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64,
	maxJobsAhead uint64,
	maxWasmFuel uint64,
	baseObjectStore dstore.Store,
	workerFactory work.WorkerFactory,
) RuntimeConfig {
	return RuntimeConfig{
		CacheSaveInterval:    cacheSaveInterval,
		SubrequestsSplitSize: subrequestsSplitSize,
		ParallelSubrequests:  parallelSubrequests,
		MaxJobsAhead:         maxJobsAhead,
		MaxWasmFuel:          maxWasmFuel,
		BaseObjectStore:      baseObjectStore,
		WorkerFactory:        workerFactory,
		// overridden by Tier Options
		ModuleExecutionTracing: false,
	}
}
