package sink

import (
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLivenessChecker_IsLive(t *testing.T) {
	nowCalls := 0
	tnow, _ := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	nowFunc := func() time.Time {
		nowCalls++
		return tnow
	}

	tests := []struct {
		clock              *pbsubstreams.Clock
		expectedResult     bool
		expectedTimeChecks int
	}{
		{testClock("1a", 1, tnow.Add(-5*time.Second)), false, 1},
		{testClock("2a", 2, tnow.Add(-4*time.Second)), false, 2},
		{testClock("3a", 3, tnow.Add(-3*time.Second)), true, 3}, //threshold reached
		{testClock("4a", 4, tnow.Add(-2*time.Second)), true, 3},
		{testClock("5a", 5, tnow.Add(-1*time.Second)), true, 3},
	}

	livenessChecker := NewDeltaLivenessChecker(3 * time.Second)
	livenessChecker.nowFunc = nowFunc

	for _, tt := range tests {
		res := livenessChecker.IsLive(tt.clock)
		if res != tt.expectedResult {
			t.Errorf("expected result %v, got %v", tt.expectedResult, res)
		}
		if nowCalls != tt.expectedTimeChecks {
			t.Errorf("expected %d time checks, got %d", tt.expectedTimeChecks, nowCalls)
		}
	}

}

func testClock(id string, num uint64, time time.Time) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{
		Id:        id,
		Number:    num,
		Timestamp: timestamppb.New(time),
	}
}
