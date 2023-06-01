package scheduler

import "github.com/streamingfast/substreams/orchestrator/work"

type StagedModules []*StageProgress // staged, alphanumerically sorted module names

type StageProgress struct {
	modules []*ModuleProgress
}

type ModuleProgress struct {
	name string

	readyUpToBlock     uint64
	mergedUpToBlock    uint64
	scheduledUpToBlock uint64

	completedJobs []*work.Job
	runningJobs   []*work.Job
}

// Algorithm for planning the Next Jobs:
// We need to start from the last stage, first segment.
