package pipeline

import (
	"fmt"
	"os"
	"strconv"
)

const defaultMaxLinearHandoffLagSegments = 2

// MaxLinearHandoffLagSegments is how many segments a production request may trail the last
// final block (rounded down to a segment), either when backprocessing completes or while
// streaming final blocks. Past that, the client is disconnected so that its reconnection
// back-processes the gap in parallel.
var MaxLinearHandoffLagSegments = parseMaxLinearHandoffLagSegments(os.Getenv("SUBSTREAMS_MAX_LINEAR_HANDOFF_LAG_SEGMENTS"))

func parseMaxLinearHandoffLagSegments(value string) uint64 {
	if value == "" {
		return defaultMaxLinearHandoffLagSegments
	}
	v, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		panic(fmt.Errorf("invalid SUBSTREAMS_MAX_LINEAR_HANDOFF_LAG_SEGMENTS %q: %w", value, err))
	}
	return v
}

// LinearHandoffLagTarget returns the block the linear handoff would be set to if the request
// was planned now, and whether the given handoff trails it by more than maxLagSegments.
func LinearHandoffLagTarget(linearHandoff, stopBlock, lastFinalBlock, segmentSize, maxLagSegments uint64) (target uint64, tooFarBehind bool) {
	target = lastFinalBlock - (lastFinalBlock % segmentSize)
	if stopBlock != 0 {
		stopBoundary := stopBlock
		if remainder := stopBlock % segmentSize; remainder != 0 {
			stopBoundary = stopBlock - remainder + segmentSize
		}
		target = min(target, stopBoundary)
	}

	if target <= linearHandoff {
		return target, false
	}
	return target, target-linearHandoff > maxLagSegments*segmentSize
}
