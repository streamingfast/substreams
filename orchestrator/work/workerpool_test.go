package work

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
)

func Test_workerPoolPool_Borrow_Return(t *testing.T) {
	ctx := context.Background()
	pi := NewWorkerPool(ctx, 2, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd {
			return func() loop.Msg {
				return &Result{}
			}
		})
	})

	assert.Len(t, pi.workers, 2)
	avail, shouldRetry := pi.WorkerAvailable()
	assert.True(t, avail)
	assert.False(t, shouldRetry)
	worker1 := pi.Borrow()

	// only one worker available until 4 seconds have passed
	avail, shouldRetry = pi.WorkerAvailable()
	assert.False(t, avail)
	assert.True(t, shouldRetry)

	// after delay, all workers are available
	newStarted := (*pi.started).Add(-5 * time.Second)
	pi.started = &newStarted

	avail, shouldRetry = pi.WorkerAvailable()
	assert.True(t, avail)
	assert.False(t, shouldRetry)
	worker2 := pi.Borrow()

	avail, shouldRetry = pi.WorkerAvailable()
	assert.False(t, avail)
	assert.False(t, shouldRetry)
	assert.Panics(t, func() { pi.Borrow() })
	pi.Return(worker2)
	avail, shouldRetry = pi.WorkerAvailable()
	assert.True(t, avail)
	assert.False(t, shouldRetry)
	pi.Return(worker1)
	assert.Panics(t, func() { pi.Return(worker1) })
}
