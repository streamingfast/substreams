package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type Scheduler struct {
	workPlan               *work.Plan
	submittedJobs          []*work.Job
	respFunc               substreams.ResponseFunc
	graph                  *manifest.ModuleGraph
	upstreamRequestModules *pbsubstreams.Modules

	OnStoreJobTerminated func(moduleName string, partialsWritten block.Ranges) error
}

func NewScheduler(workPlan *work.Plan, respFunc substreams.ResponseFunc, upstreamRequestModules *pbsubstreams.Modules) *Scheduler {
	return &Scheduler{
		workPlan:               workPlan,
		respFunc:               respFunc,
		upstreamRequestModules: upstreamRequestModules,
	}
}

type jobResult struct {
	job             *work.Job
	partialsWritten block.Ranges
	err             error
}

func fromWorkResult(job *work.Job, wr *work.Result) jobResult {
	return jobResult{
		job:             job,
		partialsWritten: wr.PartialsWritten,
		err:             wr.Error,
	}
}

func (s *Scheduler) Schedule(ctx context.Context, pool work.WorkerPool) (err error) {
	logger := reqctx.Logger(ctx)
	result := make(chan jobResult)

	wg := &sync.WaitGroup{}
	logger.Debug("launching scheduler")

	go func() {
		finished := false
		for !finished {
			finished = s.run(ctx, wg, result, pool)
		}
		logger.Info("scheduler finished scheduling jobs. waiting for jobs to complete")

		wg.Wait()
		logger.Info("all jobs completed")

		close(result)
		logger.Debug("result channel closed")

		return

	}()

	return s.gatherResults(ctx, result)
}

func (s *Scheduler) run(ctx context.Context, wg *sync.WaitGroup, result chan jobResult, pool work.WorkerPool) (finished bool) {
	worker := pool.Borrow(ctx)
	if worker == nil {
		return true
	}

	nextJob := s.getNextJob(ctx)
	if nextJob == nil {
		return true
	}

	wg.Add(1)
	s.submittedJobs = append(s.submittedJobs, nextJob)
	go func() {
		jr := s.runSingleJob(ctx, worker, nextJob, s.upstreamRequestModules)
		result <- jr

		pool.Return(worker)
		wg.Done()
	}()

	return false
}

func (s *Scheduler) getNextJob(ctx context.Context) (nextJob *work.Job) {
	for {
		if ctx.Err() != nil {
			return nil
		}
		nextJob, moreJobs := s.workPlan.NextJob()
		if nextJob != nil {
			return nextJob
		}
		if moreJobs {
			time.Sleep(1 * time.Second)
			continue
		}
		return nil
	}
}
func (s *Scheduler) gatherResults(ctx context.Context, result chan jobResult) (err error) {
	for {
		select {
		case <-ctx.Done():
			if err = ctx.Err(); err != nil {
				return err
			}
			return nil
		case jobResult, ok := <-result:
			if !ok {
				return nil
			}
			if err := s.processJobResult(jobResult); err != nil {
				return fmt.Errorf("process job result: %w", err)
			}
		}
	}
}

func (s *Scheduler) processJobResult(result jobResult) error {
	if result.err != nil {
		wp := s.workPlan.String()
		submittedJobs := "submitted jobs: \n"
		for _, sj := range s.submittedJobs {
			submittedJobs += sj.String() + "\n"
		}
		return fmt.Errorf("worker ended in error: %w\n%s\n%s", result.err, wp, submittedJobs)
	}

	if result.partialsWritten != nil {
		// This signals back to the Squasher that it can squash this segment
		if err := s.OnStoreJobTerminated(result.job.ModuleName, result.partialsWritten); err != nil {
			return fmt.Errorf("on job terminated: %w", err)
		}
	}

	return nil
}

// OnStoreCompletedUntilBlock is called to indicate that the given storeName
// has snapshots at the `storeSaveIntervals` up to `blockNum` here.
//
// This should unlock all jobs that were dependent
func (s *Scheduler) OnStoreCompletedUntilBlock(storeName string, blockNum uint64) {
	s.workPlan.MarkDependencyComplete(storeName, blockNum)
}

func (s *Scheduler) runSingleJob(ctx context.Context, worker work.Worker, job *work.Job, requestModules *pbsubstreams.Modules) jobResult {
	logger := reqctx.Logger(ctx)
	request := job.CreateRequest(requestModules)

	var workResult *work.Result
	var nonRetryableError error

	_ = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if nonRetryableError != nil {
			return nonRetryableError
		}

		workResult = worker.Work(ctx, request, s.respFunc)
		err := workResult.Error

		switch err.(type) {
		case *work.RetryableErr:
			logger.Debug("retryable error", zap.Error(err))
			return err
		default:
			if err != nil {
				logger.Debug("not a retryable error", zap.Error(err))
			}
			nonRetryableError = err
			return nil
		}
	})

	jr := fromWorkResult(job, workResult)
	return jr
}
