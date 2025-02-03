package work

import (
	"context"
)

type WorkerPool interface {
	Borrow(ctx context.Context) (Worker, error)
	Return(ctx context.Context, worker Worker)
}
