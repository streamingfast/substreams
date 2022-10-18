package config

import "github.com/streamingfast/dstore"

type RuntimeConfig struct {
	StoreSnapshotsSaveInterval uint64
	ExecOutputSaveInterval     uint64

	SubrequestsSplitSize int // in multiple of the SaveIntervals above
	ParallelSubrequests  int // how many sub-jobs to launch for a given user

	StoreSnapshotsObjectStore dstore.Store // only for `store` modules snapshots (full and partial)
	ExecOutputObjectStore     dstore.Store // execution output of both `map` and `store` module kinds
}
