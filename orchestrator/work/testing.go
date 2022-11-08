package work

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/storagestate"
	"strings"
)

func TestJob(modName string, rng string, prio int) *Job {
	return NewJob(modName, block.ParseRange(rng), nil, prio)
}

func TestPlanReadyJobs(jobs ...*Job) *Plan {
	return &Plan{
		readyJobs: jobs,
	}
}

func TestJobDeps(modName string, rng string, prio int, deps string) *Job {
	return NewJob(modName, block.ParseRange(rng), strings.Split(deps, ","), prio)
}

func TestModState(modName string, rng string) storagestate.ModuleStorageState {
	return &storagestate.StoreStorageState{ModuleName: modName, PartialsMissing: block.ParseRanges(rng)}
}

func TestModStateMap(modStates ...storagestate.ModuleStorageState) (out storagestate.ModuleStorageStateMap) {
	out = make(storagestate.ModuleStorageStateMap)
	for _, mod := range modStates {
		out[mod.Name()] = mod
	}
	return
}
