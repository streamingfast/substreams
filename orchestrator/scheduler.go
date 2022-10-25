package orchestrator

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/reqctx"
	"io"
	"sync"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Scheduler struct {
	workerPool             work.JobRunnerPool
	workPlan               *WorkPlan
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
	workPlan *WorkPlan,
	graph *manifest.ModuleGraph,
	respFunc substreams.ResponseFunc,
	logger *zap.Logger,
	upstreamRequestModules *pbsubstreams.Modules,
) (*Scheduler, error) {

	// TODO(abourget): Have the WorkerPool arrive as an INTERFACE with only Borrow() and Return()
	// so we don't even know anything about its internals.
	workerPool := work.NewWorkerPool(runtimeConfig.ParallelSubrequests, runtimeConfig.WorkerFactory, logger)

	// TODO(abourget): rework that jobsPlanner to better fit within the Scheduler, but now at
	// least it's isolated within, and no one externally knows about it.
	jobsPlanner, err := NewJobsPlanner(ctx, workPlan, runtimeConfig.SubrequestsSplitSize, graph)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	s := &Scheduler{
		workerPool:             workerPool,
		workPlan:               workPlan,
		respFunc:               respFunc,
		upstreamRequestModules: upstreamRequestModules,

		//jobsPlanner: jobsPlanner, // DEPRECATED
	}
	return s, nil
}

type jobResult struct {
	job *Job
	err error
}

func (s *Scheduler) Schedule(ctx context.Context) (err error) {
	logger := reqctx.Logger(ctx)
	result := make(chan jobResult)
	done := make(chan error, 1)
	defer func() {
		<-done
		close(result)
	}()

	logger.Debug("launching scheduler")

	go func() {
		done <- s.resultGatherer(ctx, result)
	}()

	for {
		if err := s.runOne(ctx, result); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("run one: %w", err)
		}
	}

	<-done
	close(done)
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
			if err != nil {
				return fmt.Errorf("worker ended in error: %w", err)
			}
		}
	}

}

//
//
//	go s.launch(ctx, result)
//
//	jobCount := s.jobsPlanner.JobCount()
//	for resultCount := 0; resultCount < jobCount; {
//		select {
//		case <-ctx.Done():
//			if err = ctx.Err(); err != nil {
//				return err
//			}
//			logger.Info("job canceled")
//			return nil
//		case err = <-result:
//			resultCount++
//			if err != nil {
//				err = fmt.Errorf("worker ended in error: %w", err)
//				return err
//			}
//			logger.Debug("received result", zap.Int("result_count", resultCount), zap.Int("job_count", jobCount))
//		}
//	}
//
//	logger.Info("all jobs completed, waiting for squasher to finish")
//
//	return nil
//}

func (s *Scheduler) runOne(ctx context.Context, result chan jobResult) (err error) {
	jobRunner := s.workerPool.Borrow()
	defer s.workerPool.Return(jobRunner)

	nextJob := s.workPlan.NextJob()
	if nextJob == nil {
		return io.EOF
	}

	go func() {
		// TODO: validate this is the right thing..
		err := s.runSingleJob(ctx, jobRunner, nextJob, s.upstreamRequestModules)
		result <- jobResult{job: nextJob, err: err}
	}()
	return nil
}

//
//func (s *Scheduler) launch(ctx context.Context, result chan error) {
//	logger := reqctx.Logger(ctx)
//	ctx, span := reqctx.WithSpan(ctx, "running_schedule")
//	defer span.End()
//	for {
//		logger.Debug("getting a next job from scheduler", zap.Int("available_jobs", len(s.jobsPlanner.AvailableJobs)))
//		job, ok := <-s.jobsPlanner.AvailableJobs
//		if !ok {
//			logger.Debug("no more job in scheduler, or context cancelled")
//			break
//		}
//
//		logger.Info("scheduling job", zap.Object("job", job))
//
//		start := time.Now()
//		jobRunner := s.workerPool.Borrow()
//		logger.Debug("got worker", zap.Object("job", job), zap.Duration("in", time.Since(start)))
//
//		select {
//		case <-ctx.Done():
//			logger.Info("synchronize stores quit on cancel context")
//			break
//		default:
//		}
//
//		go func() {
//			select {
//			case result <- s.runSingleJob(ctx, jobRunner, job, s.upstreamRequestModules):
//				s.workerPool.Return(jobRunner)
//			case <-ctx.Done():
//			}
//		}()
//	}
//}

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

	if partialsWritten != nil {
		// This is the back signal to the Squasher
		if err := s.OnStoreJobTerminated(job.ModuleName, partialsWritten); err != nil {
			return fmt.Errorf("squashing: %w", err)
		}
	}

	return nil
}

// TODO(abourget): JobsPlanner, to be folded into the Scheduler, hidden behind it, an implementation
// detail of the Scheduler.

type JobsPlanner struct {
	sync.Mutex

	jobs          work.JobList // all jobs, completed or not
	AvailableJobs chan *work.Job
	completed     bool
}

func NewJobsPlanner(
	ctx context.Context,
	workPlan *WorkPlan,
	subrequestSplitSize uint64,
	graph *manifest.ModuleGraph,
) (*JobsPlanner, error) {
	planner := &JobsPlanner{}

	logger := reqctx.Logger(ctx)

	for storeName, workUnit := range workPlan.fileUnitsMap {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// do nothing
		}

		requests := workUnit.batchRequests(subrequestSplitSize)
		rangeLen := len(requests)
		for idx, requestRange := range requests {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				// do nothing
			}
			ancestorStoreModules, err := graph.AncestorStoresOf(storeName)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", storeName, err)
			}

			job := work.NewJob(storeName, requestRange, ancestorStoreModules, rangeLen, idx)
			planner.jobs = append(planner.jobs, job)

			logger.Info("job planned", zap.String("module_name", storeName), zap.Uint64("start_block", requestRange.StartBlock), zap.Uint64("end_block", requestRange.ExclusiveEndBlock))
		}
	}

	planner.sortJobs()
	planner.AvailableJobs = make(chan *work.Job, len(planner.jobs))
	planner.dispatch()

	logger.Info("jobs planner ready")

	return planner, nil
}

//func (p *JobsPlanner) sortJobs() {
//	sort.Slice(p.jobs, func(i, j int) bool {
//		// reverse sorts priority, higher first
//		return p.jobs[i].Priority > p.jobs[j].Priority
//	})
//}
//
//func (p *JobsPlanner) dispatch() {
//	if p.completed {
//		return
//	}
//
//	var scheduled int
//	for _, job := range p.jobs {
//		if job.Scheduled {
//			scheduled++
//			continue
//		}
//		if job.ReadyForDispatch() {
//			job.Scheduled = true
//			p.AvailableJobs <- job
//		}
//	}
//	if scheduled == len(p.jobs) {
//		close(p.AvailableJobs)
//		p.completed = true
//	}
//}

func (p *JobsPlanner) SignalCompletionUpUntil(storeName string, blockNum uint64) {
	p.Lock()
	defer p.Unlock()

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
