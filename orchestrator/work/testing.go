package work

import (
	"strings"

	state2 "github.com/streamingfast/substreams/storage/store/state"

	"github.com/streamingfast/substreams/storage/execout/state"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage"
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

func TestStoreState(modName string, rng string) storage.ModuleStorageState {
	return &state2.StoreStorageState{ModuleName: modName, PartialsMissing: block.ParseRanges(rng)}
}

func TestMapState(modName string, rng string) storage.ModuleStorageState {
	return &state.ExecOutputStorageState{ModuleName: modName, SegmentsMissing: block.ParseRanges(rng)}
}

func TestModStateMap(modStates ...storage.ModuleStorageState) (out storage.ModuleStorageStateMap) {
	out = make(storage.ModuleStorageStateMap)
	for _, mod := range modStates {
		out[mod.Name()] = mod
	}
	return
}
