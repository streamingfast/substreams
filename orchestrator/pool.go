package orchestrator

import (
	"context"
	"fmt"
	"io"
	"sync"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type requestWaiter struct {
	Request *pbsubstreams.Request
	Waiter  Waiter
}

type Notifier interface {
	Notify(builder string, blockNum uint64)
}

type Pool struct {
	requestWaiters     []*requestWaiter
	readyRequestStream chan *requestWaiter
	requestQueue       PriorityQueue
	requestQueueMu     sync.Mutex

	waiters   map[Waiter]struct{}
	waitersMu sync.RWMutex

	readActive     bool
	readActivation sync.Once

	init sync.Once
	wg   sync.WaitGroup

	done chan struct{}
}

func NewPool() *Pool {
	p := Pool{}

	p.readyRequestStream = make(chan *requestWaiter, 5000)
	p.waiters = map[Waiter]struct{}{}
	p.done = make(chan struct{})

	p.requestQueue = make(PriorityQueue, 0)
	(&p.requestQueue).QInit()

	return &p
}

func (p *Pool) Notify(builder string, blockNum uint64) {
	p.waitersMu.RLock()
	defer p.waitersMu.RUnlock()

	for waiter := range p.waiters {
		waiter.Signal(builder, blockNum)
	}
}

func (p *Pool) Add(ctx context.Context, request *pbsubstreams.Request, waiter Waiter) error {
	if p.readActive {
		return fmt.Errorf("cannot add to pool once reading has begun")
	}

	rw := &requestWaiter{
		Request: request,
		Waiter:  waiter,
	}

	p.waitersMu.Lock()
	p.waiters[rw.Waiter] = struct{}{}
	p.requestWaiters = append(p.requestWaiters, rw)
	p.waitersMu.Unlock()

	return nil
}

func (p *Pool) Start(ctx context.Context) {
	p.init.Do(func() {
		p.wg.Add(len(p.requestWaiters))

		go func() {
			defer close(p.readyRequestStream)
			defer close(p.done)
			p.wg.Wait()
			return
		}()

		for _, rw := range p.requestWaiters {
			go func(item *requestWaiter) {
				defer p.wg.Done()

				select {
				case <-ctx.Done():
					return
				case <-item.Waiter.Wait(ctx):
					select {
					case <-ctx.Done():
						return
					case p.readyRequestStream <- item:
						p.waitersMu.Lock()
						delete(p.waiters, item.Waiter)
						p.waitersMu.Unlock()
					}
				}
			}(rw)
		}
	})

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
					p.requestQueueMu.Lock()
					(&p.requestQueue).PushRequest(i.Request, i.Waiter.Order())
					p.requestQueueMu.Unlock()
				}
			}
		}()
	})
}

func (p *Pool) Get(ctx context.Context) (*pbsubstreams.Request, error) {
	sleeper := NewSleeper(10)

	var r *pbsubstreams.Request
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.done:
			p.requestQueueMu.Lock()
			queueLen := p.requestQueue.Len()
			if queueLen > 0 {
				r = (&p.requestQueue).PopRequest()
				p.requestQueueMu.Unlock()
				if r != nil {
					return r, nil
				}
			} else {
				p.requestQueueMu.Unlock()
			}
			return nil, io.EOF
		default:
			//
		}

		p.requestQueueMu.Lock()
		if p.requestQueue.Len() == 0 {
			p.requestQueueMu.Unlock()
			sleeper.Sleep()
			continue
		}

		sleeper.Reset()

		r = (&p.requestQueue).PopRequest()
		p.requestQueueMu.Unlock()
		if r != nil {
			return r, nil
		}
	}
}
