package orchestrator

import (
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/work"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"sort"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// TODO(abourget): WorkPlan can be renamed `Plan` if lives in `work` package, below `orchestrator`

type WorkPlan struct {
	// storageState // would we need that later? perhaps not

	fileUnitsMap map[string]*FileUnits // WorksUnits split by module name

	//sortedJobDensity []string // storeName or jobs in order of speed / complexity.

	jobsCompleted   atomic.Int64
	prioritizedJobs []*work.Job
}

func (p *WorkPlan) SplitWorkIntoJobs(subrequestSplitSize uint64, graph *manifest.ModuleGraph) error {
	for storeName, workUnit := range p.fileUnitsMap {
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

			job := work.NewJob(storeName, requestRange, ancestorStoreModules, rangeLen, idx)
			planner.jobs = append(planner.jobs, job)

			logger.Info("job planned", zap.String("module_name", storeName), zap.Uint64("start_block", requestRange.StartBlock), zap.Uint64("end_block", requestRange.ExclusiveEndBlock))
		}
	}

	planner.sortJobs()
	planner.AvailableJobs = make(chan *work.Job, len(planner.jobs))
	planner.dispatch()

	logger.Info("jobs planner ready")
}

func (p *JobsPlanner) sortJobs() {
	// TODO(abourget): absorb this method in the WorkPlan
	sort.Slice(p.jobs, func(i, j int) bool {
		// reverse sorts priority, higher first
		return p.jobs[i].Priority > p.jobs[j].Priority
	})
}

func (p *JobsPlanner) dispatch() {
	// TODO(abourget): absorb this method in the WorkPlan
	if p.completed {
		return
	}

	var scheduled int
	for _, job := range p.jobs {
		if job.Scheduled {
			scheduled++
			continue
		}
		if job.ReadyForDispatch() {
			job.Scheduled = true
			p.AvailableJobs <- job
		}
	}
	if scheduled == len(p.jobs) {
		close(p.AvailableJobs)
		p.completed = true
	}
}

func (p *WorkPlan) Prioritize() {
	// mutex locked?
	// sorts prioritizedJobs
	// based on whatever is available to this WorkPlan
}

func (b *WorkPlan) NextJob() *Job {
	// TODO: fetch from the already prioritizedJobs
	// mutex locked?
}

func (p *WorkPlan) StoreCount() int {
	return len(p.fileUnitsMap)
}

func (p WorkPlan) InitialProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, unit := range p.fileUnitsMap {
		if unit.initialCompleteRange == nil {
			continue
		}

		more := []*pbsubstreams.BlockRange{
			{
				StartBlock: unit.initialCompleteRange.StartBlock,
				EndBlock:   unit.initialCompleteRange.ExclusiveEndBlock,
			},
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

func (p *WorkPlan) String() string {
	var out []string
	for k, v := range p.fileUnitsMap {
		out = append(out, fmt.Sprintf("mod=%q, initial=%s, partials missing=%v, present=%v", k, v.initialCompleteRange.String(), v.partialsMissing, v.partialsPresent))
	}
	return strings.Join(out, ";")
}
