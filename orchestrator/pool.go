package orchestrator

import (
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

	waiters map[Waiter]struct{}

	readActive     bool
	readActivation sync.Once

	init  sync.Once
	wg    sync.WaitGroup
	mutex sync.Mutex
}

func NewPool() *Pool {
	return &Pool{}
}

func (p *Pool) Notify(builder string, blockNum uint64) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

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
		p.stream = make(chan *poolItem)
		p.waiters = map[Waiter]struct{}{}

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
				zlog.Debug("added request to stream", zap.String("request modules", item.Request.Modules.String()))
			}
		}
	}()

	return nil
}

func (p *Pool) Get(ctx context.Context) (*pbsubstreams.Request, error) {
	p.readActivation.Do(func() {
		p.readActive = true
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case i, ok := <-p.stream:
		if !ok {
			return nil, io.EOF
		}
		return i.Request, nil
	}
}
