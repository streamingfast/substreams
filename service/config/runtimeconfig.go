package config

import (
	"github.com/streamingfast/substreams/orchestrator/work"

	"github.com/streamingfast/dstore"
)

// RuntimeConfig is a global configuration for the service.
// It is passed down and should not be modified unless cloned.
type RuntimeConfig struct {
	SegmentSize uint64

	MaxJobsAhead               uint64 // limit execution of depencency jobs so they don't go too far ahead of the modules that depend on them (ex: module X is 2 million blocks ahead of module Y that depends on it, we don't want to schedule more module X jobs until Y caught up a little bit)
	DefaultParallelSubrequests uint64 // how many sub-jobs to launch for a given user
	// derives substores `states/`, for `store` modules snapshots (full and partial)
	// and `outputs/` for execution output of both `map` and `store` module kinds
	BaseObjectStore dstore.Store
	DefaultCacheTag string // appended to BaseObjectStore unless overriden by auth layer
	WorkerFactory   work.WorkerFactory

	ModuleExecutionTracing bool
}

func NewTier1RuntimeConfig(
	segmentSize uint64,
	parallelSubrequests uint64,
	maxJobsAhead uint64,
	baseObjectStore dstore.Store,
	defaultCacheTag string,
	workerFactory work.WorkerFactory,
) RuntimeConfig {
	return RuntimeConfig{
		SegmentSize:                segmentSize,
		DefaultParallelSubrequests: parallelSubrequests,
		MaxJobsAhead:               maxJobsAhead,
		BaseObjectStore:            baseObjectStore,
		DefaultCacheTag:            defaultCacheTag,
		WorkerFactory:              workerFactory,
		// overridden by Tier Options
		ModuleExecutionTracing: false,
	}
}
