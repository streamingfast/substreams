package work

import (
	"context"
	"testing"
)

type TestWorkerPool struct {
	workerFactory func(ctx context.Context) Worker
}

func NewTestWorkerPool(t *testing.T, workerFactory func(ctx context.Context) Worker) *TestWorkerPool {
	return &TestWorkerPool{
		workerFactory: workerFactory,
	}
}

func (t *TestWorkerPool) Borrow(ctx context.Context) (Worker, error) {
	return t.workerFactory(ctx), nil
}

func (t *TestWorkerPool) ReleaseAll() {}

func (t *TestWorkerPool) Return(ctx context.Context, worker Worker) {
	//nada
}

func (t *TestWorkerPool) RampingUp() bool {
	return false
}
