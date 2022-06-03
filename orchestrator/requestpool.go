package orchestrator

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"
)

type requestWaiter struct {
	ReverseIndex int // per module decrementing index
	job          *Job
	Waiter       Waiter
}

type Notifier interface {
	Notify(builder string, blockNum uint64)
}

type RequestPool struct {
	readyRequestStream chan *requestWaiter

	requestWaiters []*requestWaiter

	queueSend    chan *QueueItem
	queueReceive chan *QueueItem

	waiters      map[Waiter]struct{}
	waitersMutex sync.RWMutex

	totalRequests int

	start      sync.Once
	readActive bool
	wg         sync.WaitGroup
}

func NewRequestPool() *RequestPool {
	p := RequestPool{}

	p.readyRequestStream = make(chan *requestWaiter, 5000)

	p.queueSend = make(chan *QueueItem, 5000)
	p.queueReceive = make(chan *QueueItem)

	p.waiters = map[Waiter]struct{}{}

	return &p
}

func (p *RequestPool) State() string {
	p.waitersMutex.RLock()
	defer p.waitersMutex.RUnlock()

	var waiters []string
	for w := range p.waiters {
		waiters = append(waiters, w.String())
	}

	sort.Strings(waiters)

	return strings.Join(waiters, ",")
}

func (p *RequestPool) Notify(builder string, blockNum uint64) {
	p.waitersMutex.Lock()
	defer p.waitersMutex.Unlock()

	zlog.Debug("pool: notification received", zap.String("builder", builder), zap.Uint64("block number", blockNum))

	for waiter := range p.waiters {
		waiter.Signal(builder, blockNum)
	}
}

func (p *RequestPool) Add(ctx context.Context, reverseIdx int, job *Job, waiter Waiter) error {
	if p.readActive {
		return fmt.Errorf("cannot add to pool once reading has begun")
	}

	rw := &requestWaiter{
		ReverseIndex: reverseIdx,
		job:          job,
		Waiter:       waiter,
	}

	p.waitersMutex.Lock()
	p.totalRequests++
	p.waiters[rw.Waiter] = struct{}{}
	p.requestWaiters = append(p.requestWaiters, rw)
	p.waitersMutex.Unlock()

	return nil
}

func (p *RequestPool) Start(ctx context.Context) {
	p.start.Do(func() {
		StartQueue(ctx, p.queueSend, p.queueReceive)

		p.readActive = true

		p.wg.Add(len(p.requestWaiters))

		go func() {
			defer close(p.readyRequestStream)
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
						p.waitersMutex.Lock()
						delete(p.waiters, item.Waiter)
						p.waitersMutex.Unlock()
					}
				}
			}(rw)
		}

		go func() {
			defer close(p.queueSend) //finished sending items into queue once all
			for {
				select {
				case <-ctx.Done():
					return
				case waiter, ok := <-p.readyRequestStream:
					if !ok {
						return
					}
					// the number of stores a waiter was waiting for determines its priority in the queue.
					// higher number of stores => higher priority.
					p.queueSend <- &QueueItem{
						job:      waiter.job,
						Priority: waiter.Waiter.Order() + waiter.ReverseIndex,
					}
				}
			}
		}()
	})
}

/**


  0 0 0 0 0 0
  0 1 2
  0 2 4


  6 5 4 3 2 1

  6 5 4 3 2 1
  6 6 6 .....
  6 7 8 .....

*/

func (p *RequestPool) Get(ctx context.Context) (*Job, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case qi, ok := <-p.queueReceive:
		if !ok {
			return nil, io.EOF
		}
		return qi.job, nil
	}
}

func (p *RequestPool) Count() int {
	return p.totalRequests
}
