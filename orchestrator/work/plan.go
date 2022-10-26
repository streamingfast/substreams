package work

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"strings"
	"sync"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Plan struct {
	ModulesStateMap ModuleStorageStateMap

	//sortedJobDensity []string // storeName or jobs in order of speed / complexity.

	jobsCompleted   atomic.Int64
	prioritizedJobs []*Job

	mu sync.Mutex
}

func NewPlan() *Plan {
	return &Plan{
		ModulesStateMap: make(ModuleStorageStateMap),
	}
}

func (p *Plan) Build(ctx context.Context, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval, subrequestSplitSize uint64, upToBlock uint64, graph *manifest.ModuleGraph) error {
	storageState, err := fetchStorageState(ctx, storeConfigMap)
	if err != nil {
		return fmt.Errorf("fetching stores states: %w", err)
	}

	if err := p.buildPlan(ctx, storageState, storeConfigMap, storeSnapshotsSaveInterval, upToBlock); err != nil {
		return fmt.Errorf("build plan: %w", err)
	}

	p.splitWorkIntoJobs()
	p.sendWorkPlanProgress()
}

func (p *Plan) buildPlan(ctx context.Context, storageState *StorageState, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval, upToBlock uint64) error {
	logger := reqctx.Logger(ctx)

	for _, config := range storeConfigMap {
		name := config.Name()
		snapshot, ok := storageState.Snapshots[name]
		if !ok {
			return fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}

		moduleStorageState, err := newModuleStorageState(name, storeSnapshotsSaveInterval, config.ModuleInitialBlock(), upToBlock, snapshot)
		if err != nil {
			return fmt.Errorf("new file units %q: %w", name, err)
		}

		p.ModulesStateMap[name] = moduleStorageState
	}
	logger.Info("work plan ready", zap.Stringer("work_plan", p.ModulesStateMap))
	return nil
}

func (p *Plan) splitWorkIntoJobs(subrequestSplitSize uint64, graph *manifest.ModuleGraph) error {
	for storeName, workUnit := range p.ModulesStateMap {
		requests := workUnit.batchRequests(subrequestSplitSize)
		rangeLen := len(requests)
		for idx, requestRange := range requests {
			// TODO(abourget): figure out a way to do those calls only once. Mind you, in the
			// future, we might need to re-compute the ancestor graph at different places
			// during the history of the chain, as "moduleInitialBlock"s evolve with PATCH
			// modules.
			ancestorStoreModules, err := graph.AncestorStoresOf(storeName)
			if err != nil {
				return fmt.Errorf("getting ancestore stores for %s: %w", storeName, err)
			}

			job := NewJob(storeName, requestRange, ancestorStoreModules, rangeLen, idx)
			p.prioritizedJobs = append(p.prioritizedJobs, job)
		}
	}

	// TODO(abourget): The SCHEDULER is the one
	// who will sort jobs (call Prioritize()) and then
	// GetNextJob() in the loop over there.
	// No reason to have this data-munging function do such
	// dispatches.
	planner.sortJobs()
	planner.AvailableJobs = make(chan *Job, len(planner.jobs))
	planner.dispatch()

	return nil
}

//
//func (p *orchestrator.JobsPlanner) sortJobs() {
//	// TODO(abourget): absorb this method in the work.Plan
//	sort.Slice(p.jobs, func(i, j int) bool {
//		// reverse sorts priority, higher first
//		return p.jobs[i].Priority > p.jobs[j].Priority
//	})
//}
//
//func (p *orchestrator.JobsPlanner) dispatch() {
//	// TODO(abourget): absorb this method in the work.Plan
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

func (p *Plan) Prioritize(prioritizer Prioritizer) {
	p.mu.Lock()
	defer p.mu.Unlock()

	prioritizer.Sort(p.prioritizedJobs)
}

func (p *Plan) NextJob() *Job {
	p.mu.Lock()
	defer p.mu.Unlock()

	job := p.prioritizedJobs[0]
	p.prioritizedJobs = p.prioritizedJobs[1:]

	return job
}

func (p *Plan) StoreCount() int {
	return len(p.ModulesStateMap)
}

func (p *Plan) InitialProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, unit := range p.ModulesStateMap {
		if unit.InitialCompleteRange == nil {
			continue
		}

		var more []*pbsubstreams.BlockRange
		if unit.InitialCompleteRange != nil {
			more = append(more, &pbsubstreams.BlockRange{
				StartBlock: unit.InitialCompleteRange.StartBlock,
				EndBlock:   unit.InitialCompleteRange.ExclusiveEndBlock,
			})
		}

		for _, rng := range unit.initialProcessedPartials() {
			more = append(more, &pbsubstreams.BlockRange{
				StartBlock: rng.StartBlock,
				EndBlock:   rng.ExclusiveEndBlock,
			})
		}

		out = append(out, &pbsubstreams.ModuleProgress{
			Name: storeName,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: more,
				},
			},
		})
	}
	return
}

func (p *Plan) String() string {
	var out []string
	for k, v := range p.ModulesStateMap {
		out = append(out, fmt.Sprintf("mod=%q, initial=%s, partials missing=%v, present=%v", k, v.InitialCompleteRange.String(), v.PartialsMissing, v.PartialsPresent))
	}
	return strings.Join(out, ";")
}
