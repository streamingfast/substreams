package config

import (
	"github.com/streamingfast/substreams/orchestrator/work"

	"github.com/streamingfast/dstore"
)

// RuntimeConfig is a global configuration for the service.
// It is passed down and should not be modified unless cloned.
type RuntimeConfig struct {
	StateBundleSize uint64

	MaxWasmFuel                uint64 // if not 0, enable fuel consumption monitoring to stop runaway wasm module processing forever
	MaxJobsAhead               uint64 // limit execution of depencency jobs so they don't go too far ahead of the modules that depend on them (ex: module X is 2 million blocks ahead of module Y that depends on it, we don't want to schedule more module X jobs until Y caught up a little bit)
	DefaultParallelSubrequests uint64 // how many sub-jobs to launch for a given user
	// derives substores `states/`, for `store` modules snapshots (full and partial)
	// and `outputs/` for execution output of both `map` and `store` module kinds
	BaseObjectStore dstore.Store
	DefaultCacheTag string // appended to BaseObjectStore unless overriden by auth layer
	WorkerFactory   work.WorkerFactory

	ModuleExecutionTracing bool
	MaxConcurrentRequests  int64
}

func NewTier1RuntimeConfig(
	stateBundleSize uint64,
	parallelSubrequests uint64,
	maxJobsAhead uint64,
	maxWasmFuel uint64,
	baseObjectStore dstore.Store,
	defaultCacheTag string,
	workerFactory work.WorkerFactory,
) RuntimeConfig {
	return RuntimeConfig{
		StateBundleSize:            stateBundleSize,
		DefaultParallelSubrequests: parallelSubrequests,
		MaxJobsAhead:               maxJobsAhead,
		MaxWasmFuel:                maxWasmFuel,
		BaseObjectStore:            baseObjectStore,
		DefaultCacheTag:            defaultCacheTag,
		WorkerFactory:              workerFactory,
		// overridden by Tier Options
		ModuleExecutionTracing: false,
	}
}

func NewTier2RuntimeConfig() RuntimeConfig {
	return RuntimeConfig{} //values overridden by options
}
