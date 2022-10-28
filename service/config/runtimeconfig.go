package config

import (
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/orchestrator/work"
)

type RuntimeConfig struct {
	StoreSnapshotsSaveInterval uint64
	ExecOutputSaveInterval     uint64
	SubrequestsSplitSize       uint64 // in multiple of the SaveIntervals above
	ParallelSubrequests        uint64 // how many sub-jobs to launch for a given user
	// derives substores `states/`, for `store` modules snapshots (full and partial)
	// and `outputs/` for execution output of both `map` and `store` module kinds
	BaseObjectStore  dstore.Store
	WorkerFactory    work.JobRunnerFactory
	WithRequestStats bool
}

func NewRuntimeConfig(
	storeSnapshotsSaveInterval uint64,
	execOutputSaveInterval uint64,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64,
	baseObjectStore dstore.Store,
	workerFactory work.JobRunnerFactory,
) RuntimeConfig {
	return RuntimeConfig{
		StoreSnapshotsSaveInterval: storeSnapshotsSaveInterval,
		ExecOutputSaveInterval:     execOutputSaveInterval,
		SubrequestsSplitSize:       subrequestsSplitSize,
		ParallelSubrequests:        parallelSubrequests,
		BaseObjectStore:            baseObjectStore,
		WorkerFactory:              workerFactory,
	}
}
