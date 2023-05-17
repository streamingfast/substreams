package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type in struct {
	request  *pbssinternal.ProcessRangeRequest
	respFunc substreams.ResponseFunc
}
type out struct {
	partialsWritten store.FileInfos
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
		func(_ substreams.ResponseFromAnyTier) error {
			return nil
		},
		&pbsubstreams.Modules{Modules: mods},
	)
	var accumulatedRanges block.Ranges
	sched.OnStoreJobTerminated = func(_ context.Context, mod string, partialFilesWritten store.FileInfos) error {
		assert.Equal(t, "B", mod)
		accumulatedRanges = append(accumulatedRanges, partialFilesWritten.Ranges()...)
		return nil
	}
	go func() {
		in := <-inchan
		rng := fmt.Sprintf("%d-%d", in.request.StartBlockNum, in.request.StopBlockNum)
		outchan <- out{partialsWritten: store.PartialFiles(rng)}

		in = <-inchan
		rng = fmt.Sprintf("%d-%d", in.request.StartBlockNum, in.request.StopBlockNum)
		outchan <- out{partialsWritten: store.PartialFiles(rng)}
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
			return work.NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *work.Result {
				inchan <- in{request, respFunc}
				out := <-outchan
				return &work.Result{
					PartialFilesWritten: out.partialsWritten,
					Error:               out.err,
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
		parallelism      uint64
		expectMoreJobs   bool
		expectPoolLength int
	}{
		{
			name:        "3 jobs,3 workers",
			parallelism: 3,
			plan: work.TestPlanReadyJobs(
				work.TestJob("A", "0-10", 1),
				work.TestJob("A", "10-20", 2),
				work.TestJob("A", "20-30", 3),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Scheduler{
				workPlan:    test.plan,
				currentJobs: make(map[string]*work.Job),
			}
			wg := &sync.WaitGroup{}
			result := make(chan jobResult, 100)
			pool := testNoopRunnerPool(test.parallelism)

			assert.False(t, s.run(context.Background(), wg, result, pool))
			assert.False(t, s.run(context.Background(), wg, result, pool))
			assert.False(t, s.run(context.Background(), wg, result, pool))
			assert.True(t, s.run(context.Background(), wg, result, pool))

			time.Sleep(10 * time.Millisecond) // give small amount of time to the goroutine to finish
			assert.Len(t, result, 3)
		})
	}
}

func testNoopRunnerPool(parallelism uint64) work.WorkerPool {
	ctx := context.Background()
	runnerPool := work.NewWorkerPool(ctx, parallelism,
		func(logger *zap.Logger) work.Worker {
			return work.NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *work.Result {
				return &work.Result{}
			})
		},
	)
	return runnerPool
}
