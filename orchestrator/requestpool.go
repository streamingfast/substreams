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
	zlog.Debug("starting request pool")
	p.start.Do(func() {
		zlog.Debug("starting queue")
		StartQueue(ctx, p.queueSend, p.queueReceive)

		p.readActive = true

		wg := sync.WaitGroup{}

		zlog.Debug("adding to waitgroup", zap.Int("waiters", len(p.requestWaiters)))
		wg.Add(len(p.requestWaiters))

		go func() {
			wg.Wait()
			zlog.Debug("done waiting for waitgroup")
			close(p.readyRequestStream)
		}()

		for _, rw := range p.requestWaiters {
			go func(item *requestWaiter) {
				defer wg.Done()

				select {
				case <-ctx.Done():
					return
				case <-item.Waiter.Wait(ctx):
					select {
					case <-ctx.Done():
						return
					case p.readyRequestStream <- item:
						zlog.Debug("sent to ready request stream")
						p.waitersMutex.Lock()
						delete(p.waiters, item.Waiter)
						p.waitersMutex.Unlock()
					}
				}
			}(rw)
		}

		go func() {
			defer func() {
				zlog.Debug("closing queue send")
				close(p.queueSend) //finished sending items into queue once all
			}()
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
					zlog.Debug("sending in queue")
					p.queueSend <- &QueueItem{
						job:      waiter.job,
						Priority: waiter.Waiter.Size() + waiter.ReverseIndex,
					}
					zlog.Debug("sent to queue")
				}
			}
		}()
	})
}

func (p *RequestPool) GetNext(ctx context.Context) (*Job, error) {
	zlog.Debug("in GetNext")
	select {
	case <-ctx.Done():
		return nil, nil
	case qi, ok := <-p.queueReceive:
		zlog.Debug("got something from queue")
		if !ok {
			return nil, io.EOF
		}
		return qi.job, nil
	}
}

func (p *RequestPool) Count() int {
	return p.totalRequests
}
