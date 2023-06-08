package work

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator/loop"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func Test_workerPoolPool_Borrow_Return(t *testing.T) {
	ctx := context.Background()
	pi := NewWorkerPool(ctx, 2, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) loop.Cmd {
			return func() loop.Msg {
				return &Result{}
			}
		})
	})

	assert.Len(t, pi.workers, 2)
	assert.True(t, pi.WorkerAvailable())
	worker1 := pi.Borrow()
	assert.True(t, pi.WorkerAvailable())
	worker2 := pi.Borrow()
	assert.False(t, pi.WorkerAvailable())
	assert.Panics(t, func() { pi.Borrow() })
	pi.Return(worker2)
	assert.True(t, pi.WorkerAvailable())
	pi.Return(worker1)
	assert.Panics(t, func() { pi.Return(worker1) })
}
