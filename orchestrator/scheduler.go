package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	respFunc               substreams.ResponseFunc
	graph                  *manifest.ModuleGraph
	upstreamRequestModules *pbsubstreams.Modules

	OnStoreJobTerminated func(moduleName string, partialsWritten block.Ranges) error

	// TODO(abourget): deprecate this, and fuse it inside the Scheduler
	//jobsPlanner *JobsPlanner
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

func (s *Scheduler) Schedule(ctx context.Context, pool work.JobRunnerPool) (err error) {
	logger := reqctx.Logger(ctx)
	result := make(chan jobResult)

	wg := &sync.WaitGroup{}
	logger.Debug("launching scheduler")

	go func() {
		for {
			if !s.runOne(ctx, wg, result, pool) {
				wg.Wait()
				close(result)
				return
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
		return fmt.Errorf("worker ended in error: %w", result.err)
	}
	if result.partialsWritten != nil {
		// This signals back to the Squasher that it can squash this segment
		if err := s.OnStoreJobTerminated(result.job.ModuleName, result.partialsWritten); err != nil {
			return fmt.Errorf("on job terminated: %w", err)
		}
	}
	return nil
}

func (s *Scheduler) runOne(ctx context.Context, wg *sync.WaitGroup, result chan jobResult, pool work.JobRunnerPool) (moreJobs bool) {
	jobRunner := pool.Borrow()
	defer pool.Return(jobRunner)

	nextJob, moreJobs := s.workPlan.NextJob()
	if nextJob == nil {
		// TODO(colin): who closes the `result` channel?
		if moreJobs {
			time.Sleep(1 * time.Second)
			return true
		}
		return false
	}

	wg.Add(1)
	go func() {
		partialsWritten, err := s.runSingleJob(ctx, jobRunner, nextJob, s.upstreamRequestModules)
		result <- jobResult{job: nextJob, partialsWritten: partialsWritten, err: err}
		wg.Done()
	}()
	return true
}

// OnStoreCompletedUntilBlock is called to say the given storeName
// has snapshots at the `storeSaveIntervals` up to `blockNum` here.
//
// This should unlock all jobs that were dependent
func (s *Scheduler) OnStoreCompletedUntilBlock(storeName string, blockNum uint64) {
	s.workPlan.MarkDependencyComplete(storeName, blockNum)
}

func (s *Scheduler) runSingleJob(ctx context.Context, jobRunner work.JobRunner, job *work.Job, requestModules *pbsubstreams.Modules) (partialsWritten block.Ranges, err error) {
	logger := reqctx.Logger(ctx)
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
		return nil, err
	}
	return
}

//
//func (p *JobsPlanner) JobCount() int {
//	return len(p.jobs)
//}
//
//func (p *JobsPlanner) MarshalLogObject(enc zapcore.ObjectEncoder) error {
//	enc.AddArray("jobs", p.jobs)
//	enc.AddBool("completed", p.completed)
//	enc.AddInt("available_jobs", len(p.AvailableJobs))
//	return nil
//}
//
////
