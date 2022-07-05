package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type JobsPlanner struct {
	sync.Mutex

	jobs          jobList // all jobs, completed or not
	AvailableJobs chan *Job
	completed     bool
}

func NewJobsPlanner(
	ctx context.Context,
	workPlan WorkPlan,
	subrequestSplitSize uint64,
	stores map[string]*state.Store,
	graph *manifest.ModuleGraph,
) (*JobsPlanner, error) {
	planner := &JobsPlanner{}

	for modName, workUnit := range workPlan {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// do nothing
		}

		store := stores[modName]

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
			ancestorStoreModules, err := graph.AncestorStoresOf(store.Name)
			if err != nil {
				return nil, fmt.Errorf("getting ancestore stores for %s: %w", store.Name, err)
			}

			job := NewJob(store.Name, requestRange, ancestorStoreModules, rangeLen, idx)
			planner.jobs = append(planner.jobs, job)

			zlog.Info("job planned", zap.String("module_name", store.Name), zap.Uint64("start_block", requestRange.StartBlock), zap.Uint64("end_block", requestRange.ExclusiveEndBlock))
		}
	}

	planner.sortJobs()
	planner.AvailableJobs = make(chan *Job, len(planner.jobs))
	planner.dispatch()

	zlog.Info("jobs planner ready", zap.Object("planner", planner))

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

// func (s *OrderedStrategy) getRequestStream(ctx context.Context) <-chan *Job {
// 	requestsStream := make(chan *Job)
// 	go func() {
// 		defer close(requestsStream)

// 		for {
// 			job, err := s.requestPool.GetNext(ctx)
// 			if err == io.EOF {
// 				zlog.Debug("EOF in getRequestStream")
// 				return
// 			}
// 			select {
// 			case <-ctx.Done():
// 				zlog.Debug("ctx cannnlaskdfjlkj")
// 				return
// 			case requestsStream <- job:
// 			}
// 		}
// 	}()
// 	return requestsStream
// }
