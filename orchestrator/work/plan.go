package work

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
	"sort"
	"strings"
	"sync"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Plan struct {
	ModulesStateMap ModuleStorageStateMap

	upToBlock uint64

	waitingJobs []*Job
	readyJobs   []*Job

	modulesReadyUpToBlock map[string]uint64

	mu sync.Mutex
}

func BuildNewPlan(ctx context.Context, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval, subrequestSplitSize, upToBlock uint64, graph *manifest.ModuleGraph) (*Plan, error) {
	plan := &Plan{
		ModulesStateMap: make(ModuleStorageStateMap),
		upToBlock:       upToBlock,
	}
	storageState, err := fetchStorageState(ctx, storeConfigMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}
	if err := plan.buildPlanFromStorageState(ctx, storageState, storeConfigMap, storeSnapshotsSaveInterval, upToBlock); err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	if err := plan.splitWorkIntoJobs(subrequestSplitSize, graph); err != nil {
		return nil, fmt.Errorf("split to jobs: %w", err)
	}
	plan.initModulesReadyUpToBlock()
	return plan, nil
}

func (p *Plan) buildPlanFromStorageState(ctx context.Context, storageState *StorageState, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval, upToBlock uint64) error {
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
	logger.Info("work plan ready", zap.Stringer("work_plan", p))
	return nil
}

func (p *Plan) splitWorkIntoJobs(subrequestSplitSize uint64, graph *manifest.ModuleGraph) error {
	highestJobOrdinal := int(p.upToBlock / subrequestSplitSize)
	for storeName, workUnit := range p.ModulesStateMap {
		requests := workUnit.batchRequests(subrequestSplitSize)
		for _, requestRange := range requests {
			ancestorStoreModules, err := graph.AncestorStoresOf(storeName)
			if err != nil {
				return fmt.Errorf("getting ancestor stores for %s: %w", storeName, err)
			}

			requiredModules := moduleNames(ancestorStoreModules)
			jobOrdinal := int(requestRange.StartBlock / subrequestSplitSize)
			priority := highestJobOrdinal - jobOrdinal - len(requiredModules)

			job := NewJob(storeName, requestRange, requiredModules, priority)
			p.waitingJobs = append(p.waitingJobs, job)
		}
	}
	return nil
}

func (p *Plan) initModulesReadyUpToBlock() {
	p.modulesReadyUpToBlock = make(map[string]uint64)
	for modName, modState := range p.ModulesStateMap {
		if modState.InitialCompleteRange == nil {
			p.modulesReadyUpToBlock[modName] = modState.ModuleInitialBlock
		} else {
			p.modulesReadyUpToBlock[modName] = modState.InitialCompleteRange.ExclusiveEndBlock
		}
	}
	p.promoteWaitingJobs()
	p.prioritize()
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

func (p *Plan) prioritize() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// TODO(abourget): TEST THIS, that the priority calculation is now better
	sort.Slice(p.readyJobs, func(i, j int) bool {
		// reverse sorts priority, higher first
		return p.readyJobs[i].priority > p.readyJobs[j].priority
	})
}

func (p *Plan) MarkDependencyComplete(modName string, upToBlock uint64) {
	current := p.modulesReadyUpToBlock[modName]
	if upToBlock > current {
		p.modulesReadyUpToBlock[modName] = upToBlock
	}
	p.promoteWaitingJobs()
	p.prioritize()
}

func (p *Plan) promoteWaitingJobs() {
	p.mu.Lock()
	defer p.mu.Unlock()

	removeJobs := map[*Job]bool{}
	for _, job := range p.waitingJobs {
		if p.allDependenciesMet(job) {
			p.readyJobs = append(p.readyJobs, job)
			removeJobs[job] = true
		}
	}
	if len(removeJobs) != 0 {
		var newWaitingJobs []*Job
		for _, job := range p.waitingJobs {
			if !removeJobs[job] {
				newWaitingJobs = append(newWaitingJobs, job)
			}
		}
		p.waitingJobs = newWaitingJobs
	}
}

func (p *Plan) allDependenciesMet(job *Job) bool {
	startBlock := job.RequestRange.StartBlock
	for _, dep := range job.requiredModules {
		if p.modulesReadyUpToBlock[dep] < startBlock {
			return false
		}
	}
	return true
}

func (p *Plan) NextJob() (job *Job, more bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.readyJobs) == 0 {
		return nil, p.hasMore()
	}

	job = p.readyJobs[0]
	p.readyJobs = p.readyJobs[1:]
	return job, p.hasMore()
}

func (p *Plan) hasMore() bool {
	return len(p.readyJobs)+len(p.waitingJobs) > 0
}

func (p *Plan) SendInitialProgressMessages(respFunc substreams.ResponseFunc) error {
	progressMessages := p.initialProgressMessages()
	if err := respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return err
	}
	return nil
}

func (p *Plan) initialProgressMessages() (out []*pbsubstreams.ModuleProgress) {
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

func moduleNames(modules []*pbsubstreams.Module) (out []string) {
	for _, mod := range modules {
		out = append(out, mod.Name)
	}
	return
}
