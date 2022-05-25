package orchestrator

import (
	"context"
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"io"
	"sync"
)

type requestWaiter struct {
	Request *pbsubstreams.Request
	Waiter  Waiter
}

type Notifier interface {
	Notify(builder string, blockNum uint64)
}

type Pool struct {
	readyRequestStream chan *requestWaiter
	requestQueue       PriorityQueue

	waiters map[Waiter]struct{}

	readActive     bool
	readActivation sync.Once

	init  sync.Once
	wg    sync.WaitGroup
	mutex sync.RWMutex

	done chan struct{}
}

func NewPool() *Pool {
	return &Pool{}
}

func (p *Pool) Notify(builder string, blockNum uint64) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	for waiter := range p.waiters {
		waiter.Signal(builder, blockNum)
	}
}

func (p *Pool) Add(ctx context.Context, request *pbsubstreams.Request, waiter Waiter) error {
	if p.readActive {
		return fmt.Errorf("cannot add to pool once reading has begun")
	}

	p.wg.Add(1)

	item := &requestWaiter{
		Request: request,
		Waiter:  waiter,
	}

	p.init.Do(func() {
		p.readyRequestStream = make(chan *requestWaiter, 5000)
		p.waiters = map[Waiter]struct{}{}
		p.done = make(chan struct{})

		p.requestQueue = make(PriorityQueue, 0)
		(&p.requestQueue).QInit()

		go func() {
			defer close(p.readyRequestStream)
			defer close(p.done)
			p.wg.Wait()
			return
		}()
	})

	p.mutex.Lock()
	p.waiters[item.Waiter] = struct{}{}
	p.mutex.Unlock()

	go func() {
		defer p.wg.Done()

		select {
		case <-ctx.Done():
			return
		case <-item.Waiter.Wait(ctx):
			select {
			case <-ctx.Done():
				return
			case p.readyRequestStream <- item:
				p.mutex.Lock()
				delete(p.waiters, item.Waiter)
				p.mutex.Unlock()
				zlog.Debug("added request to readyRequestStream", zap.String("request modules", item.Request.Modules.String()))
			}
		}
	}()

	return nil
}

func (p *Pool) Get(ctx context.Context) (*pbsubstreams.Request, error) {
	p.readActivation.Do(func() {
		p.readActive = true

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-p.readyRequestStream:
					if !ok {
						return
					}
					// the number of stores a waiter was waiting for determines its priority in the queue.
					// higher number of stores => higher priority.
					(&p.requestQueue).PushRequest(i.Request, i.Waiter.Order())
				}
			}
		}()
	})

	sleeper := NewSleeper(10)

	var r *pbsubstreams.Request
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.done:
			if len(p.requestQueue) > 0 {
				r = (&p.requestQueue).PopRequest()
				if r != nil {
					return r, nil
				}
			}
			return nil, io.EOF
		default:
			//
		}

		if len(p.requestQueue) == 0 {
			sleeper.Sleep()
			continue
		}

		sleeper.Reset()

		r = (&p.requestQueue).PopRequest()
		if r != nil {
			return r, nil
		}
	}
}
