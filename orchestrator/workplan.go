package orchestrator

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *Backprocessor) buildWorkPlan() (out *WorkPlan, err error) {
	storageState, err := fetchStorageState(b.ctx, b.storeConfigs)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	out = &WorkPlan{
		workUnitsMap: map[string]*WorkUnits{},  // per module
	}

	for _, config := range b.storeConfigs {
		name := config.Name()
		snapshot, ok := storageState.Snapshots[name]
		if !ok {
			return nil, fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}
		// TODO(abourget): Pass in the `SaveInterval` in some ways
		out.workUnitsMap[name] = SplitWork(name, b.runtimeConfig.StoreSnapshotsSaveInterval, config.ModuleInitialBlock(), upToBlock, snapshot)
	}
	b.log.Info("work plan ready", zap.Stringer("work_plan", out))


	return
}

type WorkPlan struct {
	// storageState // would we need that later? perhaps not

	workUnitsMap map[string]*WorkUnits // WorksUnits split by module name

	prioritizedJobs []*Job
}

func (p *WorkPlan) StoreCount() int {
	return len(p.workMap)
}

// WorkPlan would not have `dispatch`, or an event handler like "CompletedUpUntil"
// It is WORKED ON by the Schduler. The scheduler would call "GiveMeMyNextJob()" and dispatch it.
// WorkPlan is acted upon, initialized, and updated when new data comes in, but not reactive.

func (p *WorkPlan) SquashPartialsPresent(squasher *MultiSquasher) error {
	// TODO(abourget): This belogns to the `NewSquasher()` or `Squasher::Init()`, based on
	// its input of `workPlan`.
	for _, w := range p {
		if w.partialsPresent.Len() == 0 {
			continue
		}
		err := squasher.Squash(w.modName, w.partialsPresent)
		if err != nil {
			return fmt.Errorf("squash partials present for module %s: %w", w.modName, err)
		}
	}
	return nil
}

func (p WorkPlan) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, unit := range p {
		if unit.initialCompleteRange == nil {
			continue
		}

		var more []*pbsubstreams.BlockRange
		if unit.initialCompleteRange != nil {
			more = append(more, &pbsubstreams.BlockRange{
				// FIXME(abourget): we'll use opentelemetry tracing for that!
				StartBlock: unit.initialCompleteRange.StartBlock,
				EndBlock:   unit.initialCompleteRange.ExclusiveEndBlock,
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

func (p WorkPlan) String() string {
	var out []string
	for k, v := range p {
		out = append(out, fmt.Sprintf("mod=%q, initial=%s, partials missing=%v, present=%v", k, v.initialCompleteRange, v.partialsMissing, v.partialsPresent))
	}
	return strings.Join(out, ";")
}
