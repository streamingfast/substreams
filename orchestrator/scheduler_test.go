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
	"sort"
	"sync"
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
	plan := work.TestPlanReadyJobs(
		work.TestJob("B", "0-10", 1),
		work.TestJob("B", "10-20", 0),
	)
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

	sort.Sort(accumulatedRanges)
	assert.Equal(t,
		block.ParseRanges("0-10,10-20").String(),
		accumulatedRanges.String(),
	)
}

func testRunnerPool(parallelism int) (work.WorkerPool, chan in, chan out) {
	inchan := make(chan in)
	outchan := make(chan out)
	ctx := context.Background()
	runnerPool := work.NewWorkerPool(ctx, 1,
		func(logger *zap.Logger) work.Worker {
			return work.WorkerFunc(func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *work.Result {
				inchan <- in{request, respFunc}
				out := <-outchan
				return &work.Result{
					PartialsWritten: out.partialsWritten,
					Error:           out.err,
				}
			})
		},
	)
	return runnerPool, inchan, outchan
}

func TestScheduler_runOne(t *testing.T) {
	tests := []struct {
		name             string
		plan             *work.Plan
		expectMoreJobs   bool
		expectPoolLength int
	}{
		{
			plan: work.TestPlanReadyJobs(
				work.TestJob("A", "0-10", 1),
				work.TestJob("A", "10-20", 1),
				work.TestJob("A", "20-30", 1),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Scheduler{workPlan: test.plan}
			wg := &sync.WaitGroup{}
			result := make(chan jobResult, 100)
			pool := testNoopRunnerPool(2)

			assert.False(t, s.run(context.Background(), wg, result, pool))
			assert.False(t, s.run(context.Background(), wg, result, pool))
			assert.False(t, s.run(context.Background(), wg, result, pool))
			assert.True(t, s.run(context.Background(), wg, result, pool))
			assert.Len(t, result, 3)
		})
	}
}

func testNoopRunnerPool(parallelism int) work.WorkerPool {
	ctx := context.Background()
	runnerPool := work.NewWorkerPool(ctx, 1,
		func(logger *zap.Logger) work.Worker {
			return work.WorkerFunc(func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *work.Result {
				return &work.Result{}
			})
		},
	)
	return runnerPool
}
