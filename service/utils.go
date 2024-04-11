package service

import (
	"sort"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func sortClocksDistributor(clockDistributor map[uint64]*pbsubstreams.Clock) (sortedClockDistributor []*pbsubstreams.Clock) {
	sortedClockDistributor = make([]*pbsubstreams.Clock, 0, len(clockDistributor))
	for _, clock := range clockDistributor {
		sortedClockDistributor = append(sortedClockDistributor, clock)
	}

	sort.Slice(sortedClockDistributor, func(i, j int) bool { return sortedClockDistributor[i].Number < sortedClockDistributor[j].Number })
	return
}

func irreversibleCursorFromClock(clock *pbsubstreams.Clock) *bstream.Cursor {
	return &bstream.Cursor{
		Step:      bstream.StepNewIrreversible,
		Block:     bstream.NewBlockRef(clock.Id, clock.Number),
		LIB:       bstream.NewBlockRef(clock.Id, clock.Number),
		HeadBlock: bstream.NewBlockRef(clock.Id, clock.Number),
	}
}
