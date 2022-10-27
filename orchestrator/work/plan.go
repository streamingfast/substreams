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
	plan.promoteWaitingJobs()
	plan.prioritize()
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

		logger.Info("work plan for store module", zap.Object("work", moduleStorageState))
	}

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
}

func (p *Plan) MarkDependencyComplete(modName string, upToBlock uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.bumpModuleUpToBlock(modName, upToBlock)
	p.promoteWaitingJobs()
	p.prioritize()
}

func (p *Plan) bumpModuleUpToBlock(modName string, upToBlock uint64) {
	// Called with locked mutex
	current := p.modulesReadyUpToBlock[modName]
	if upToBlock > current {
		p.modulesReadyUpToBlock[modName] = upToBlock
	}
}

func (p *Plan) promoteWaitingJobs() {
	// Called with locked mutex
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

func (p *Plan) prioritize() {
	// Called with locked mutex
	// TODO(abourget): TEST THIS, that the priority calculation is now better
	sort.Slice(p.readyJobs, func(i, j int) bool {
		// reverse sorts priority, higher first
		return p.readyJobs[i].priority > p.readyJobs[j].priority
	})
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
	for storeName, modState := range p.ModulesStateMap {
		var more []*pbsubstreams.BlockRange
		if modState.InitialCompleteRange != nil {
			more = append(more, &pbsubstreams.BlockRange{
				StartBlock: modState.InitialCompleteRange.StartBlock,
				EndBlock:   modState.InitialCompleteRange.ExclusiveEndBlock,
			})
		}

		for _, rng := range modState.PartialsPresent.Merged() {
			more = append(more, &pbsubstreams.BlockRange{
				StartBlock: rng.StartBlock,
				EndBlock:   rng.ExclusiveEndBlock,
			})
		}

		if len(more) != 0 {
			out = append(out, &pbsubstreams.ModuleProgress{
				Name: storeName,
				Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
					ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
						ProcessedRanges: more,
					},
				},
			})
		}
	}
	return
}
func moduleNames(modules []*pbsubstreams.Module) (out []string) {
	for _, mod := range modules {
		out = append(out, mod.Name)
	}
	return
}
