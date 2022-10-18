package orchestrator

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WorkPlan struct {
	// storageState // would we need that later? perhaps not

	workUnitsMap map[string]*WorkUnits // WorksUnits split by module name

	prioritizedJobs []*Job
}

func (p *WorkPlan) StoreCount() int {
	return len(p.workUnitsMap)
}

func (p WorkPlan) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, unit := range p.workUnitsMap {
		if unit.initialCompleteRange == nil {
			continue
		}

		var more []*pbsubstreams.BlockRange
		if unit.initialCompleteRange != nil {
			more = append(more, &pbsubstreams.BlockRange{
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
	for k, v := range p.workUnitsMap {
		out = append(out, fmt.Sprintf("mod=%q, initial=%s, partials missing=%v, present=%v", k, v.initialCompleteRange, v.partialsMissing, v.partialsPresent))
	}
	return strings.Join(out, ";")
}
