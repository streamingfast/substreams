package work

import (
	"context"
	"errors"
)

var ErrorResourceExhausted = errors.New("resource exhausted")
var ErrorResourceExhaustedRampUp = errors.New("resource exhausted during ramp up")

type WorkerPool interface {
	Borrow(ctx context.Context) (Worker, error)
	Return(ctx context.Context, worker Worker)
}
