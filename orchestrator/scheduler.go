package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.opentelemetry.io/otel"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Scheduler struct {
	workerPool *WorkerPool
	respFunc   substreams.ResponseFunc

	OnJobTermianted func(moduleName string, partialsWritten []*block.Range) error

	squasher      *MultiSquasher
	availableJobs <-chan *Job
	tracer        ttrace.Tracer
}

func NewScheduler(ctx context.Context, workPlan *WorkPlan, squasher *MultiSquasher, workerPool *WorkerPool, respFunc substreams.ResponseFunc) (*Scheduler, error) {
	tracer := otel.GetTracerProvider().Tracer("scheduler")

	jobsPlanner, err := NewJobsPlanner(p.reqCtx, workPlan, uint64(p.subrequestSplitSize), p.graph)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	s := &Scheduler{
		squasher:      squasher,
		availableJobs: jobsPlanner.AvailableJobs,
		workerPool:    workerPool,
		respFunc:      respFunc,
		tracer:        tracer,
	}
	return s, nil
}

func (s *Scheduler) Launch(ctx context.Context, requestModules *pbsubstreams.Modules, result chan error) {
	ctx, span := s.tracer.Start(ctx, "running_schedule")
	defer span.End()
	for {
		zlog.Debug("getting a next job from scheduler", zap.Int("available_jobs", len(s.availableJobs)))
		job, ok := <-s.availableJobs
		if !ok {
			zlog.Debug("no more job in scheduler, or context cancelled")
			break
		}

		zlog.Info("scheduling job", zap.Object("job", job))

		start := time.Now()
		jobWorker := s.workerPool.Borrow()
		zlog.Debug("got worker", zap.Object("job", job), zap.Duration("in", time.Since(start)))

		select {
		case <-ctx.Done():
			zlog.Info("synchronize stores quit on cancel context")
			break
		default:
		}

		go func() {
			select {
			case result <- s.runSingleJob(ctx, jobWorker, job, requestModules):
			case <-ctx.Done():
			}
		}()
	}
}

func (s *Scheduler) OnStoreCompletedUntilBlock(storeName string, blockNum uint64) {
	// This replaces the JobPlanner's signaling mechanism: allows decoupling from the Squasher and the Scheduler
	//	func (p *JobsPlanner) SignalCompletionUpUntil(storeName string, blockNum uint64) {

	// TODO(abourget): is it only for `storeName` or `moduleName` can be used when we want parallel processing of
	// exec output?

}

func (s *Scheduler) runSingleJob(ctx context.Context, worker Worker, job *Job, requestModules *pbsubstreams.Modules) error {
	var partialsWritten []*block.Range
	var err error

out:
	for i := 0; uint64(i) < 3; i++ {
		partialsWritten, err = worker.Run(ctx, job, requestModules, s.respFunc)

		switch err.(type) {
		case *RetryableErr:
			zlog.Debug("retryable error", zap.Error(err))
			continue
		default:
			if err != nil {
				zlog.Debug("not a retryable error", zap.Error(err))
			}
			break out
		}
	}

	s.workerPool.ReturnWorker(worker)

	if err != nil {
		return err
	}

	if partialsWritten != nil {
		// This is the back signal to the Squasher
		if err := s.OnJobTerminated(job.ModuleName, partialsWritten); err != nil {
			return fmt.Errorf("squashing: %w", err)
		}
	}

	return nil
}

// TODO(abourget): JobsPlanner, to be folded into the Scheduler, hidden behind it, an implementation
// detail of the Scheduler.

type JobsPlanner struct {
	sync.Mutex

	jobs          jobList // all jobs, completed or not
	AvailableJobs chan *Job
	completed     bool
	tracer        ttrace.Tracer
}

func NewJobsPlanner(
	ctx context.Context,
	workPlan WorkPlan,
	subrequestSplitSize uint64,
	graph *manifest.ModuleGraph,
) (*JobsPlanner, error) {
	planner := &JobsPlanner{
		tracer: otel.GetTracerProvider().Tracer("executor"),
	}

	ctx, span := planner.tracer.Start(ctx, "job_planning")
	defer span.End()

	for storeName, workUnit := range workPlan {
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
			// TODO(abourget): here we loop WorkUnit.reqChunks, and grab the ancestor modules
			// to setup the waiter.
			// blockRange's start/end come from `requestRange`
			ancestorStoreModules, err := graph.AncestorStoresOf(storeName)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", storeName, err)
			}

			job := NewJob(storeName, requestRange, ancestorStoreModules, rangeLen, idx)
			planner.jobs = append(planner.jobs, job)

			zlog.Info("job planned", zap.String("module_name", storeName), zap.Uint64("start_block", requestRange.StartBlock), zap.Uint64("end_block", requestRange.ExclusiveEndBlock))
		}
	}

	planner.sortJobs()
	planner.AvailableJobs = make(chan *Job, len(planner.jobs))
	planner.dispatch()

	zlog.Info("jobs planner ready")

	return planner, nil
}

func (p *JobsPlanner) sortJobs() {
	sort.Slice(p.jobs, func(i, j int) bool {
		// reverse sorts priority, higher first
		return p.jobs[i].priority > p.jobs[j].priority
	})
}

func (p *JobsPlanner) SignalCompletionUpUntil(storeName string, blockNum uint64) {
	p.Lock()
	defer p.Unlock()

	for _, job := range p.jobs {
		if job.scheduled {
			continue
		}

		job.signalDependencyResolved(storeName, blockNum)
	}

	p.dispatch()
}

func (p *JobsPlanner) dispatch() {
	zlog.Debug("calling jobs planner dispatch", zap.Object("planner", p))
	if p.completed {
		return
	}

	var scheduled int
	for _, job := range p.jobs {
		if job.scheduled {
			scheduled++
			continue
		}
		if job.readyForDispatch() {
			job.scheduled = true
			zlog.Debug("dispatching job", zap.Object("job", job))
			p.AvailableJobs <- job
		}
	}
	if scheduled == len(p.jobs) {
		close(p.AvailableJobs)
		p.completed = true
	}
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
