package sink

import (
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type LivenessChecker interface {
	IsLive(block *pbsubstreams.Clock) bool
}

type DeltaLivenessChecker struct {
	delta   time.Duration
	nowFunc func() time.Time

	isLive bool
}

func NewDeltaLivenessChecker(delta time.Duration) *DeltaLivenessChecker {
	return &DeltaLivenessChecker{
		delta:   delta,
		nowFunc: time.Now,
	}
}

func (t *DeltaLivenessChecker) IsLive(clock *pbsubstreams.Clock) bool {
	if t.isLive {
		return true
	}

	if clock == nil {
		return false
	}

	blockTimeStamp := clock.GetTimestamp()
	if blockTimeStamp == nil {
		return false
	}

	blockTime := blockTimeStamp.AsTime()
	nowTime := t.nowFunc()

	if nowTime.Sub(blockTime) <= t.delta {
		t.isLive = true
	}

	return t.isLive
}
