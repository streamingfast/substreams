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

	currentJobsLock sync.Mutex
	currentJobs     map[string]*work.Job

	OnStoreJobTerminated func(ctx context.Context, moduleName string, partialsWritten block.Ranges) error
}

func NewScheduler(workPlan *work.Plan, respFunc substreams.ResponseFunc, upstreamRequestModules *pbsubstreams.Modules) *Scheduler {
	return &Scheduler{
		workPlan:               workPlan,
		respFunc:               respFunc,
		upstreamRequestModules: upstreamRequestModules,
		currentJobs:            make(map[string]*work.Job),
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
		allJobsStarted := false
		for !allJobsStarted {
			allJobsStarted = s.run(ctx, wg, result, pool)
		}
		logger.Info("scheduler finished starting jobs. waiting for them to complete")

		wg.Wait()
		logger.Info("all jobs completed")

		close(result)
		logger.Debug("result channel closed")
	}()

	return s.gatherResults(ctx, result)
}

func jobsSummary(jobs map[string]*work.Job) (out []string) {
	for k, j := range jobs {
		out = append(out, fmt.Sprintf("%s (on %s,%d:%d)", j.ModuleName, k, j.RequestRange.StartBlock, j.RequestRange.ExclusiveEndBlock))
	}
	return
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
	s.currentJobsLock.Lock()
	reqctx.Logger(ctx).Debug("current running jobs", zap.Strings("jobs", jobsSummary(s.currentJobs)))
	s.currentJobs[worker.ID()] = nextJob
	s.currentJobsLock.Unlock()
	go func() {
		jr := s.runSingleJob(ctx, worker, nextJob, s.upstreamRequestModules)
		select {
		case <-ctx.Done():
		case result <- jr:
		}
		s.currentJobsLock.Lock()
		delete(s.currentJobs, worker.ID())
		s.currentJobsLock.Unlock()

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
			if err := s.processJobResult(ctx, jobResult); err != nil {
				return fmt.Errorf("process job result for target %q: %w", jobResult.job.ModuleName, err)
			}
		}
	}
}

func (s *Scheduler) processJobResult(ctx context.Context, result jobResult) error {
	if result.err != nil {
		return fmt.Errorf("worker ended in error: %w", result.err)
	}

	if result.partialsWritten != nil {
		// This signals back to the Squasher that it can squash this segment
		if err := s.OnStoreJobTerminated(ctx, result.job.ModuleName, result.partialsWritten); err != nil {
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

	if err := ctx.Err(); err != nil {
		logger.Info("job not completed", zap.Object("job", job), zap.Error(err))
		return jobResult{err: err}
	}

	jr := fromWorkResult(job, workResult)
	logger.Info("job completed", zap.Object("job", job), zap.Error(workResult.Error))
	return jr
}
