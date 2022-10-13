package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJobsPlanner(t *testing.T) {
	t.Skip("abourget: incomplete, untested")

	subreqSplit := uint64(100)
	mods := manifest.NewTestModules()
	graph, err := manifest.NewModuleGraph(mods)
	require.NoError(t, err)

	storeMods, err := graph.StoresDownTo([]string{"G"})
	require.NoError(t, err)

	mockDStore := dstore.NewMockStore(nil)

	//{}(storeSplit, 0, 0, mockDStore, zlog)
	storeMap := store.NewMap()
	for _, mod := range storeMods {
		kindStore := mod.Kind.(*pbsubstreams.Module_KindStore_).KindStore

		newStore, err := store.NewFullKV(mod.Name, mod.InitialBlock, "myhash", kindStore.UpdatePolicy, kindStore.ValueType, mockDStore, zlog)
		require.NoError(t, err)

		storeMap.Set(mod.Name, newStore)
	}

	splitWorkMods := WorkPlan{
		"A": &WorkUnit{modName: "A"},
		"B": &WorkUnit{modName: "B"},
		"C": &WorkUnit{modName: "C"},
		"D": &WorkUnit{modName: "D"},
		"E": &WorkUnit{modName: "E"},
		"F": &WorkUnit{modName: "F"},
		"G": &WorkUnit{modName: "G"},
		"H": &WorkUnit{modName: "H"},
		"K": &WorkUnit{modName: "K"},
	}

	ctx := context.Background()
	s, err := NewJobsPlanner(
		ctx,
		splitWorkMods,
		subreqSplit,
		graph,
	)
	require.NoError(t, err)

	s.SignalCompletionUpUntil("E", 10)
	s.SignalCompletionUpUntil("E", 20)
	s.SignalCompletionUpUntil("B", 20)

	var allRequests []string

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for req := range s.AvailableJobs {
		fmt.Println(jobstr(req))
		allRequests = append(allRequests, jobstr(req))
	}

	fmt.Println(allRequests)

	assert.Equal(t, 8, len(allRequests))
}

func Test_OrderedJobsPlanner(t *testing.T) {
	modules := []*pbsubstreams.Module{
		{
			Name:         "A",
			InitialBlock: uint64(0),
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		},
		{
			Name:         "B",
			InitialBlock: uint64(0),
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "A",
					}},
				},
			},
		},
	}

	graph, err := manifest.NewModuleGraph(modules)
	require.NoError(t, err)

	workPlan := WorkPlan{
		"A": &WorkUnit{
			modName: "A",
			partialsMissing: block.Ranges{
				&block.Range{
					StartBlock:        uint64(0),
					ExclusiveEndBlock: uint64(100),
				},
				&block.Range{
					StartBlock:        uint64(100),
					ExclusiveEndBlock: uint64(200),
				},
				&block.Range{
					StartBlock:        uint64(300),
					ExclusiveEndBlock: uint64(400),
				},
				&block.Range{
					StartBlock:        uint64(400),
					ExclusiveEndBlock: uint64(500),
				},
				&block.Range{
					StartBlock:        uint64(500),
					ExclusiveEndBlock: uint64(600),
				},
			},
		},
		"B": &WorkUnit{
			modName: "B",
			partialsMissing: block.Ranges{
				&block.Range{
					StartBlock:        uint64(0),
					ExclusiveEndBlock: uint64(100),
				},
			},
		},
	}

	ctx := context.Background()
	jobsPlanner, err := NewJobsPlanner(
		ctx,
		workPlan,
		uint64(100),
		graph,
	)
	require.NoError(t, err)
	close(jobsPlanner.AvailableJobs)

	for job := range jobsPlanner.AvailableJobs {
		require.NotEqual(t, "B", job.ModuleName)
	}
}

func jobstr(j *Job) string {
	return fmt.Sprintf("%s %d-%d", j.ModuleName, j.requestRange.StartBlock, j.requestRange.ExclusiveEndBlock)
}
