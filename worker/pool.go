package worker

import "context"

type Pool struct {
	c chan struct{}
}

func NewPool(size int) *Pool {
	ch := make(chan struct{}, size)

	//prime the channel with objects
	for i := 0; i < size; i++ {
		select {
		case ch <- struct{}{}:
			//
		default:
			//
		}
	}

	pool := &Pool{c: ch}
	return pool
}

func (p *Pool) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.c:
		return nil
	}
}

func (p *Pool) Done() {
	select {
	case p.c <- struct{}{}:
		//
	default:
		//
	}
}
