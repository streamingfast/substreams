package service

import (
	"os"
	"sort"
	"strconv"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
	"go.uber.org/zap"
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

func setSubstreamsStoreSizeLimitFromEnv(logger *zap.Logger) {
	if limit := os.Getenv("SUBSTREAMS_STORE_SIZE_LIMIT"); limit != "" {
		if parsed, err := strconv.ParseUint(limit, 10, 64); err == nil {
			logger.Info("using SUBSTREAMS_STORE_SIZE_LIMIT from env var", zap.Uint64("limit", parsed))
			store.StoreSizeLimit = parsed
		} else {
			logger.Warn("invalid SUBSTREAMS_STORE_SIZE_LIMIT env var", zap.String("string", limit))
		}
	}
}
