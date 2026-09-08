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
	// ReleaseAll returns every worker still borrowed. Called when the parallel processing
	// ends, before the request releases its session: the session pool cannot account for
	// workers returned once their session is gone.
	ReleaseAll()
}
