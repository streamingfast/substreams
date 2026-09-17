package pipeline

import (
	"context"
	"errors"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinearHandoffLagTarget(t *testing.T) {
	tests := []struct {
		name           string
		linearHandoff  uint64
		stopBlock      uint64
		lastFinalBlock uint64
		maxLagSegments uint64
		expectTarget   uint64
		expectTooFar   bool
	}{
		{"final block did not move", 1000, 0, 1050, 2, 1000, false},
		{"exactly max lag", 1000, 0, 1250, 2, 1200, false},
		{"one segment over max lag", 1000, 0, 1300, 2, 1300, true},
		{"stop block caps the target", 1000, 1150, 5000, 2, 1200, false},
		{"stop block above lag", 1000, 1301, 5000, 2, 1400, true},
		{"final block below handoff", 1000, 0, 900, 2, 900, false},
		{"zero max lag", 1000, 0, 1100, 0, 1100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, tooFar := LinearHandoffLagTarget(tt.linearHandoff, tt.stopBlock, tt.lastFinalBlock, 100, tt.maxLagSegments)
			assert.Equal(t, tt.expectTarget, target)
			assert.Equal(t, tt.expectTooFar, tooFar)
		})
	}
}

func TestParseMaxLinearHandoffLagSegments(t *testing.T) {
	assert.Equal(t, uint64(defaultMaxLinearHandoffLagSegments), parseMaxLinearHandoffLagSegments(""))
	assert.Equal(t, uint64(5), parseMaxLinearHandoffLagSegments("5"))
	assert.Panics(t, func() { parseMaxLinearHandoffLagSegments("abc") })
}

func TestCheckFinalBlockLag(t *testing.T) {
	newPipe := func(stopBlock, lastFinalBlock uint64, getErr error) (*Pipeline, *int) {
		calls := 0
		pipe := &Pipeline{
			ctx:             reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{StopBlockNum: stopBlock}),
			stateBundleSize: 100,
		}
		WithFinalBlockLagCheck(func() (uint64, error) {
			calls++
			return lastFinalBlock, getErr
		}, 1000)(pipe)
		return pipe, &calls
	}

	t.Run("no check inside the handoff segment", func(t *testing.T) {
		pipe, calls := newPipe(0, 5000, nil)
		require.NoError(t, pipe.checkFinalBlockLag(1050))
		assert.Equal(t, 0, *calls)
	})

	t.Run("checks once per segment, with skipped boundary block", func(t *testing.T) {
		pipe, calls := newPipe(0, 1350, nil)
		require.NoError(t, pipe.checkFinalBlockLag(1102))
		require.NoError(t, pipe.checkFinalBlockLag(1150))
		assert.Equal(t, 1, *calls)
	})

	t.Run("exactly max lag", func(t *testing.T) {
		pipe, _ := newPipe(0, 1350, nil)
		require.NoError(t, pipe.checkFinalBlockLag(1100))
	})

	t.Run("more than max lag disconnects", func(t *testing.T) {
		pipe, _ := newPipe(0, 1400, nil)
		require.ErrorIs(t, pipe.checkFinalBlockLag(1100), ErrShuttingDown)
	})

	t.Run("stop block caps the target", func(t *testing.T) {
		pipe, _ := newPipe(1250, 5000, nil)
		require.NoError(t, pipe.checkFinalBlockLag(1100))
	})

	t.Run("final block error is ignored", func(t *testing.T) {
		pipe, _ := newPipe(0, 5000, errors.New("hub not ready"))
		require.NoError(t, pipe.checkFinalBlockLag(1100))
	})

	t.Run("reached through handleStepFinal", func(t *testing.T) {
		pipe, _ := newPipe(0, 1400, nil)
		engine, err := cache.NewEngine(pipe.ctx, nil, "test.Block", nil, nil)
		require.NoError(t, err)
		pipe.execOutputCache = engine
		pipe.forkHandler = NewForkHandler()

		require.NoError(t, pipe.handleStepFinal(&pbsubstreams.Clock{Number: 1050}))
		require.ErrorIs(t, pipe.handleStepFinal(&pbsubstreams.Clock{Number: 1100}), ErrShuttingDown)
	})

	t.Run("disabled without option", func(t *testing.T) {
		pipe := &Pipeline{stateBundleSize: 100}
		require.NoError(t, pipe.checkFinalBlockLag(99999))
	})
}
