package orchestrator

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

type in struct {
	request  *pbsubstreams.Request
	respFunc substreams.ResponseFunc
}
type out struct {
	partialsWritten []*block.Range
	err             error
}

func TestSchedulerInOut(t *testing.T) {
	runnerPool, inchan, outchan := testRunnerPool(2)
	mods := manifest.NewTestModules()
	plan := work.TestPlanReadyJobs([]*Job{
		work.TestJob(),
	})
	sched := NewScheduler(
		plan,
		func(resp *pbsubstreams.Response) error {
			return nil
		},
		&pbsubstreams.Modules{Modules: mods},
	)
	var accumulatedRanges block.Ranges
	sched.OnStoreJobTerminated = func(mod string, partialsWritten block.Ranges) error {
		assert.Equal(t, "B", mod)
		accumulatedRanges = append(accumulatedRanges, partialsWritten...)
		return nil
	}
	go func() {
		in := <-inchan
		rng := fmt.Sprintf("%d-%d", in.request.StartBlockNum, in.request.StopBlockNum)
		outchan <- out{partialsWritten: block.ParseRanges(rng)}

		in = <-inchan
		rng = fmt.Sprintf("%d-%d", in.request.StartBlockNum, in.request.StopBlockNum)
		outchan <- out{partialsWritten: block.ParseRanges(rng)}
	}()

	assert.NoError(t, sched.Schedule(context.Background(), runnerPool))

	assert.Equal(t, "[0, 20)", accumulatedRanges.Merged().String())
}

func testRunnerPool(parallelism int) (work.JobRunnerPool, chan in, chan out) {
	inchan := make(chan in)
	outchan := make(chan out)
	ctx := context.Background()
	runnerPool := work.NewJobRunnerPool(ctx, 2,
		func(logger *zap.Logger) work.JobRunner {
			return func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
				inchan <- in{request, respFunc}
				out := <-outchan
				return out.partialsWritten, out.err
			}
		},
	)
	return runnerPool, inchan, outchan
}

func mkJob(modName string, rng string, prio int) *Job {
	return work.NewJob(modName, block.ParseRange(rng), nil, prio)
}

func mkJobDeps(modName string, rng string, prio int, deps string) *Job {
	return work.NewJob(modName, block.ParseRange(rng), deps, prio)
}

func mkModState(modName string, rng string) *work.ModuleStorageState {
	return &work.ModuleStorageState{ModuleName: modName, PartialsMissing: block.ParseRanges(rng)}
}

func mkModStateMap(modStates ...*work.ModuleStorageState) (out work.ModuleStorageStateMap) {
	out = make(work.ModuleStorageStateMap)
	for _, mod := range modStates {
		out[mod.ModuleName] = mod
	}
	return
}
