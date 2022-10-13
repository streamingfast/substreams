package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/streamingfast/substreams/manifest"
	"go.opentelemetry.io/otel"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
