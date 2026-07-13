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

// EnvEthCallFallbackToLatestDuration is the environment variable name that, when set,
// enables a non-deterministic fallback to latest for Ethereum eth_call/eth_getBalance.
// Its presence requires callers to acknowledge non-determinism via the
// X-substreams-acknowledge-non-deterministic header.
const EnvEthCallFallbackToLatestDuration = "ETH_CALL_FALLBACK_TO_LATEST_DURATION"

// EnvEthCallUseBlockNumberDuration is the environment variable name that, when set,
// enables using block number instead of block hash for Ethereum eth_call/eth_getBalance after a specified duration.
// This is to be used on chains with deterministic behavior but with an archive node that needs the block number to route old queries.
const EnvEthCallUseBlockNumberDuration = "ETH_CALL_USE_BLOCK_NUMBER_DURATION"

// EnvDeterministicErrorMaxAge is the environment variable name that controls how long a
// cached deterministic error is honored. Past this duration the error file is discarded
// (deleted) on read and execution is retried. Defaults to 1h. Value is a Go duration string.
const EnvDeterministicErrorMaxAge = "SUBSTREAMS_DETERMINISTIC_ERROR_MAX_AGE"

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
			store.DefaultStoreSizeLimit = parsed
		} else {
			logger.Warn("invalid SUBSTREAMS_STORE_SIZE_LIMIT env var", zap.String("string", limit))
		}
	}
}
