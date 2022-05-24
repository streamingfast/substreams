package orchestrator

import (
	"context"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"io"
	"sync"
)

type PoolItem struct {
	Request *pbsubstreams.Request
	Waiter  *Waiter
}

type Pool struct {
	stream chan *PoolItem

	waiters map[*Waiter]struct{}

	init  sync.Once
	wg    sync.WaitGroup
	mutex sync.Mutex
}

func (p *Pool) Notify(builder string, blockNum uint64) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for waiter := range p.waiters {
		waiter.Signal(builder, blockNum)
	}
}

func (p *Pool) Add(ctx context.Context, item *PoolItem) {
	p.wg.Add(1)

	p.init.Do(func() {
		p.stream = make(chan *PoolItem)
		p.waiters = map[*Waiter]struct{}{}

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
		case <-item.Waiter.Done(ctx):
			select {
			case <-ctx.Done():
				return
			case p.stream <- item:
				//done!
			}
		}
	}()
}

func (p *Pool) Get(ctx context.Context) (*pbsubstreams.Request, error) {
	select {
	case <-ctx.Done():
		return nil, nil //todo(colin): is this correct?
	case i, ok := <-p.stream:
		if !ok {
			return nil, io.EOF
		}
		return i.Request, nil
	}
}
