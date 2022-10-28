package work

import (
	"github.com/streamingfast/substreams/block"
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

func TestModState(modName string, rng string) *ModuleStorageState {
	return &ModuleStorageState{ModuleName: modName, PartialsMissing: block.ParseRanges(rng)}
}

func TestModStateMap(modStates ...*ModuleStorageState) (out ModuleStorageStateMap) {
	out = make(ModuleStorageStateMap)
	for _, mod := range modStates {
		out[mod.ModuleName] = mod
	}
	return
}
