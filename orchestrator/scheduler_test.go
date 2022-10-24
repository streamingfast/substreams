package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
	"github.com/streamingfast/substreams/work"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSchedulerInOut(t *testing.T) {
	type in struct {
		request  *pbsubstreams.Request
		respFunc substreams.ResponseFunc
	}
	type out struct {
		partialsWritten []*block.Range
		err             error
	}

	inchan := make(chan in)
	outchan := make(chan out)
	ctx := context.Background()

	mods := manifest.NewTestModules()
	graph, err := manifest.NewModuleGraph(mods)
	require.NoError(t, err)
	sched, err := NewScheduler(
		ctx,
		config.RuntimeConfig{
			WorkerFactory: func(logger *zap.Logger) work.JobRunner {
				return func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
					inchan <- in{request, respFunc}
					out := <-outchan
					return out.partialsWritten, out.err
				}
			},
			ParallelSubrequests:        2,
			SubrequestsSplitSize:       20,
			ExecOutputSaveInterval:     10,
			StoreSnapshotsSaveInterval: 10,
		},
		&WorkPlan{workUnitsMap: map[string]*WorkUnits{
			"A": {modName: "A"},
			"B": {
				modName:         "B",
				partialsMissing: block.ParseRanges("0-10,10-20"),
			},
			"C": {modName: "C"},
			"D": {modName: "D"},
			"E": {modName: "E"},
			"F": {modName: "F"},
			"G": {modName: "G"},
			"H": {modName: "H"},
			"K": {modName: "K"},
		}},
		graph,
		func(resp *pbsubstreams.Response) error {
			return nil
		},
		zap.NewNop(),
		&pbsubstreams.Modules{Modules: mods},
	)
	require.NoError(t, err)
	assert.NotNil(t, sched.workerPool)
	assert.NotNil(t, sched.jobsPlanner)
	planner := sched.jobsPlanner

	var accumulatedRanges block.Ranges
	sched.OnStoreJobTerminated = func(mod string, partialsWritten block.Ranges) error {
		assert.Equal(t, "B", mod)
		accumulatedRanges = append(accumulatedRanges, partialsWritten...)
		return nil
	}
	assert.Equal(t, 1, len(planner.jobs))

	go func() {
		in := <-inchan
		rng := fmt.Sprintf("%d-%d", in.request.StartBlockNum, in.request.StopBlockNum)
		outchan <- out{partialsWritten: block.ParseRanges(rng)}

		in = <-inchan
		rng = fmt.Sprintf("%d-%d", in.request.StartBlockNum, in.request.StopBlockNum)
		outchan <- out{partialsWritten: block.ParseRanges(rng)}
	}()

	assert.NoError(t, sched.Run(ctx))
	assert.Equal(t, "[0, 20)", accumulatedRanges.Merged().String())
}

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
		config, err := store.NewConfig(mod.Name, mod.InitialBlock, "myhash", kindStore.UpdatePolicy, kindStore.ValueType, mockDStore)
		require.NoError(t, err)
		newStore := config.NewFullKV(zap.NewNop())
		storeMap.Set(newStore)
	}

	splitWorkMods := &WorkPlan{workUnitsMap: map[string]*WorkUnits{
		"A": {modName: "A"},
		"B": {modName: "B"},
		"C": {modName: "C"},
		"D": {modName: "D"},
		"E": {modName: "E"},
		"F": {modName: "F"},
		"G": {modName: "G"},
		"H": {modName: "H"},
		"K": {modName: "K"},
	}}

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

	workPlan := &WorkPlan{workUnitsMap: map[string]*WorkUnits{
		"A": &WorkUnits{
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
		"B": &WorkUnits{
			modName: "B",
			partialsMissing: block.Ranges{
				&block.Range{
					StartBlock:        uint64(0),
					ExclusiveEndBlock: uint64(100),
				},
			},
		},
	}}

	ctx := context.Background()
	jobsPlanner, err := NewJobsPlanner(
		ctx,
		workPlan,
		100,
		graph,
	)
	require.NoError(t, err)
	close(jobsPlanner.AvailableJobs)

	for job := range jobsPlanner.AvailableJobs {
		require.NotEqual(t, "B", job.ModuleName)
	}
}

func jobstr(j *work.Job) string {
	return fmt.Sprintf("%s %d-%d", j.ModuleName, j.RequestRange.StartBlock, j.RequestRange.ExclusiveEndBlock)
}
