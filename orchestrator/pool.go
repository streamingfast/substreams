package orchestrator

import (
	"container/heap"
	"context"
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"io"
	"sync"
)

type poolItem struct {
	Request *pbsubstreams.Request
	Waiter  Waiter
}

type Notifier interface {
	Notify(builder string, blockNum uint64)
}

type Pool struct {
	stream chan *poolItem
	queue  PriorityQueue

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

	item := &poolItem{
		Request: request,
		Waiter:  waiter,
	}

	p.init.Do(func() {
		p.stream = make(chan *poolItem, 5000)
		p.waiters = map[Waiter]struct{}{}
		p.done = make(chan struct{})
		p.queue = make(PriorityQueue, 0)
		heap.Init(&p.queue)

		go func() {
			defer close(p.stream)
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
			case p.stream <- item:
				p.mutex.Lock()
				delete(p.waiters, item.Waiter)
				p.mutex.Unlock()
				zlog.Debug("added request to stream", zap.String("request modules", item.Request.Modules.String()))
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
				case i, ok := <-p.stream:
					if !ok {
						close(p.done)
						return
					}
					(&p.queue).PushRequest(i.Request, i.Waiter.Order())
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
			if len(p.queue) > 0 {
				r = (&p.queue).PopRequest()
				if r != nil {
					return r, nil
				}
			}
			return nil, io.EOF
		default:
			//
		}

		if len(p.queue) == 0 {
			sleeper.Sleep()
			continue
		}

		sleeper.Reset()

		r = (&p.queue).PopRequest()
		if r != nil {
			return r, nil
		}
	}
}
