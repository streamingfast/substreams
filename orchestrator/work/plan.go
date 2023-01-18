package work

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/pipeline/outputmodules"

	"github.com/streamingfast/substreams/storage"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Plan struct {
	ModulesStateMap storage.ModuleStorageStateMap

	upToBlock uint64

	waitingJobs []*Job
	readyJobs   []*Job

	modulesReadyUpToBlock map[string]uint64

	mu     sync.Mutex
	logger *zap.Logger
}

func BuildNewPlan(ctx context.Context, modulesStateMap storage.ModuleStorageStateMap, subrequestSplitSize, upToBlock uint64, outputGraph *outputmodules.Graph) (*Plan, error) {
	logger := reqctx.Logger(ctx)
	plan := &Plan{
		ModulesStateMap: modulesStateMap,
		upToBlock:       upToBlock,
		logger:          logger,
	}

	if err := plan.splitWorkIntoJobs(subrequestSplitSize, outputGraph.SchedulableModuleNames(), outputGraph.OutputModule().Name, outputGraph.AncestorsFrom); err != nil {
		return nil, fmt.Errorf("split to jobs: %w", err)
	}

	plan.initModulesReadyUpToBlock()
	plan.promoteWaitingJobs()
	plan.prioritize()

	return plan, nil
}

func (p *Plan) splitWorkIntoJobs(subrequestSplitSize uint64, schedulableModules []string, outputModuleName string, ancestorsFrom func(string) []string) error {

	stepSize := calculateHighestDependencyDepth(schedulableModules, p.ModulesStateMap, ancestorsFrom)
	highestJobOrdinal := int(p.upToBlock/subrequestSplitSize) * stepSize

	for _, storeName := range schedulableModules {
		modState := p.ModulesStateMap[storeName]
		if modState == nil {
			continue
		}
		requests := modState.BatchRequests(subrequestSplitSize)
		for _, requestRange := range requests {
			requiredModules := ancestorsFrom(storeName)
			dependencyDepth := ancestorsDepth(storeName, ancestorsFrom)

			jobOrdinal := int(requestRange.StartBlock/subrequestSplitSize) * stepSize
			priority := highestJobOrdinal - jobOrdinal - (dependencyDepth - 1)
			if storeName == outputModuleName {
				priority += stepSize // always run our outputModule 1 step ahead of its dependencies, it only needs the previous stores to be completed and should start ahead
			}

			p.logger.Debug("adding job",
				zap.String("module", storeName),
				zap.Uint64("start_block", requestRange.StartBlock),
				zap.Uint64("end_block", requestRange.ExclusiveEndBlock),
				zap.Int("dependencyDepth", dependencyDepth),
				zap.Int("priority", priority),
			)

			job := NewJob(storeName, requestRange, requiredModules, priority)
			p.waitingJobs = append(p.waitingJobs, job)
		}
	}

	// Loop through `mappers` and schedule them, separately from the stores
	// ModulesStateMap would be concerned ONLY with Stores
	// and we add a MapperStateMap, concerned only with Mappers
	// with the appropriate ranges in there, and not the
	// store-specific `PartialsMissing`, `PartialsPresent`, etc..
	return nil
}

func ancestorsDepth(moduleName string, ancestorsFrom func(string) []string) int {
	deepest := 1
	for _, ancestor := range ancestorsFrom(moduleName) {
		depth := 1 + ancestorsDepth(ancestor, ancestorsFrom)
		if depth > deepest {
			deepest = depth
		}
	}
	return deepest
}

func calculateHighestDependencyDepth(
	schedulableModules []string,
	modulesStateMap storage.ModuleStorageStateMap,
	ancestorsFrom func(string) []string,
) int {
	highestDependencyDepth := 1
	for _, storeName := range schedulableModules {
		if modulesStateMap[storeName] == nil {
			continue
		}
		dependencyDepth := ancestorsDepth(storeName, ancestorsFrom)
		if dependencyDepth > highestDependencyDepth {
			highestDependencyDepth = dependencyDepth
		}
	}
	return highestDependencyDepth
}

func (p *Plan) initModulesReadyUpToBlock() {
	p.modulesReadyUpToBlock = make(map[string]uint64)
	for modName, modState := range p.ModulesStateMap {
		p.modulesReadyUpToBlock[modName] = modState.ReadyUpToBlock()
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
		depUpTo, ok := p.modulesReadyUpToBlock[dep]
		if !ok || depUpTo < startBlock {
			return false
		}
	}
	return true
}

func (p *Plan) prioritize() {
	// Called with locked mutex
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
		for _, rng := range modState.InitialProgressRanges() {
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

func (p *Plan) String() string {
	workingPlan := "working plan: \n"
	waitingJobs := "waiting jobs: \n"
	readyJobs := "ready jobs: \n"
	for _, w := range p.waitingJobs {
		waitingJobs += w.String() + "\n"
	}
	for _, r := range p.readyJobs {
		readyJobs += r.String() + "\n"
	}

	return workingPlan + waitingJobs + readyJobs
}
