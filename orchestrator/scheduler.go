package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Scheduler struct {
	workerPool             work.JobRunnerPool
	workPlan               *work.Plan
	respFunc               substreams.ResponseFunc
	graph                  *manifest.ModuleGraph
	upstreamRequestModules *pbsubstreams.Modules

	OnStoreJobTerminated func(moduleName string, partialsWritten block.Ranges) error

	// TODO(abourget): deprecate this, and fuse it inside the Scheduler
	//jobsPlanner *JobsPlanner
}

func NewScheduler(
	ctx context.Context,
	runtimeConfig config.RuntimeConfig,
	workPlan *work.Plan,
	graph *manifest.ModuleGraph,
	respFunc substreams.ResponseFunc,
	upstreamRequestModules *pbsubstreams.Modules,
) (*Scheduler, error) {
	logger := reqctx.Logger(ctx)

	// TODO(abourget): Have the WorkerPool arrive as an INTERFACE with only Borrow() and Return()
	// so we don't even know anything about its internals.
	workerPool := work.NewWorkerPool(runtimeConfig.ParallelSubrequests, runtimeConfig.WorkerFactory, logger)

	// TODO(abourget): rework that jobsPlanner to better fit within the Scheduler, but now at
	// least it's isolated within, and no one externally knows about it.
	//jobsPlanner, err := NewJobsPlanner(ctx, workPlan, runtimeConfig.SubrequestsSplitSize)
	//if err != nil {
	//	return nil, fmt.Errorf("creating strategy: %w", err)
	//}
	//
	s := &Scheduler{
		workPlan:               workPlan,
		respFunc:               respFunc,
		upstreamRequestModules: upstreamRequestModules,
	}
	return s, nil
}

type jobResult struct {
	job *Job
	err error
}

func (s *Scheduler) Schedule(ctx context.Context, pool work.JobRunnerPool) (err error) {
	logger := reqctx.Logger(ctx)
	result := make(chan jobResult)
	done := make(chan error, 1)
	defer func() {
		<-done
		close(result)
	}()

	logger.Debug("launching scheduler")

	go func() {
		for s.runOne(ctx, result, pool) {
		}
		}
	}()

	return s.resultGatherer(ctx, result)
}

func (s *Scheduler) resultGatherer(ctx context.Context, result chan jobResult) (err error) {
	for {
		select {
		case <-ctx.Done():
			if err = ctx.Err(); err != nil {
				return err
			}
			return nil
		case err, ok := <-result:
			if !ok {
				return nil
			}
			resultCount++

			// TODO(abourget): we've got a result,
			// do the dispatching of stuff in a single goroutine?
			// Prioritize() ?
			// Call the Squasher messaging and all?

			if err != nil {
				return fmt.Errorf("worker ended in error: %w", err)
			}
		}
	}
}

func (s *Scheduler) runOne(ctx context.Context, result chan jobResult, pool work.JobRunnerPool) (moreJobs bool) {
	jobRunner := pool.Borrow()
	defer pool.Return(jobRunner)

	nextJob := s.workPlan.NextJob()
	if nextJob == nil {
		// TODO(colin): who closes the `result` channel?
		return false
	}

	go func() {
		// TODO: validate this is the right thing..
		err := s.runSingleJob(ctx, jobRunner, nextJob, s.upstreamRequestModules)
		result <- jobResult{job: nextJob, err: err}
		// TODO: signal that this job is finished, `wg.Done()`
	}()
	return true
}

func (s *Scheduler) OnStoreCompletedUntilBlock(storeName string, blockNum uint64) {
	// This replaces the JobPlanner's signaling mechanism: allows decoupling from the Squasher and the Scheduler
	//	func (p *JobsPlanner) SignalCompletionUpUntil(storeName string, blockNum uint64) {
	s.jobsPlanner.SignalCompletionUpUntil(storeName, blockNum)

	// TODO(abourget): is it only for `storeName` or `moduleName` can be used when we want parallel processing of
	// exec output?

}

func (s *Scheduler) runSingleJob(ctx context.Context, jobRunner work.JobRunner, job *work.Job, requestModules *pbsubstreams.Modules) (err error) {
	logger := reqctx.Logger(ctx)

	var partialsWritten []*block.Range
	newRequest := job.CreateRequest(requestModules)

out:
	for i := 0; uint64(i) < 3; i++ {
		t0 := time.Now()
		logger.Info("running job", zap.Object("job", job))
		partialsWritten, err = jobRunner(ctx, newRequest, s.respFunc)
		logger.Info("job completed", zap.Object("job", job), zap.Duration("in", time.Since(t0)))
		switch err.(type) {
		case *work.RetryableErr:
			logger.Debug("retryable error", zap.Error(err))
			continue
		default:
			if err != nil {
				logger.Debug("not a retryable error", zap.Error(err))
			}
			break out
		}
	}

	if err != nil {
		return err
	}

	// TODO(abourget): this ought to be moved to the caller, and perhaps
	// have `partialsWritten` to be returned, in a `jobResult` object???
	if partialsWritten != nil {
		// This is the back signal to the Squasher
		if err := s.OnStoreJobTerminated(job.ModuleName, partialsWritten); err != nil {
			return fmt.Errorf("squashing: %w", err)
		}
	}

	return nil
}

// SignalCompletionUpUntil is a message received from the Squasher, signaling
// that the FullKV store is ready, and so scheduling a job that depends
// on it will be okay.
func (p *JobsPlanner) SignalCompletionUpUntil(storeName string, blockNum uint64) {
	p.Lock()
	defer p.Unlock()

	// TODO: re-prioritize here, and let the Scheduler
	// unblock on Borrow()

	for _, job := range p.jobs {
		if job.Scheduled {
			continue
		}

		job.SignalDependencyResolved(storeName, blockNum)
	}

	p.dispatch()
}

func (p *JobsPlanner) JobCount() int {
	return len(p.jobs)
}

func (p *JobsPlanner) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddArray("jobs", p.jobs)
	enc.AddBool("completed", p.completed)
	enc.AddInt("available_jobs", len(p.AvailableJobs))
	return nil
}

//
