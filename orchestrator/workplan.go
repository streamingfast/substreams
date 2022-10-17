package orchestrator

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WorkPlan map[string]*WorkUnit

func (p WorkPlan) SquashPartialsPresent(squasher *Squasher) error {
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
