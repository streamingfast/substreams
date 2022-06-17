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

type jobWaiter struct {
	ReverseIndex int // per module decrementing index

	job    *Job
	Waiter Waiter
}

type Notifier interface {
	Notify(builder string, blockNum uint64)
}

type JobPool struct {
	readyJobStream chan *jobWaiter

	jobWaiters []*jobWaiter

	queueSend    chan *QueueItem
	queueReceive chan *QueueItem

	waiters      map[Waiter]struct{}
	waitersMutex sync.RWMutex

	totalJobs int

	start      sync.Once
	readActive bool
}

func NewJobPool() *JobPool {
	p := JobPool{}
	p.readyJobStream = make(chan *jobWaiter)
	p.queueSend = make(chan *QueueItem)
	p.queueReceive = make(chan *QueueItem)
	p.waiters = map[Waiter]struct{}{}
	return &p
}

func (p *JobPool) State() string {
	p.waitersMutex.RLock()
	defer p.waitersMutex.RUnlock()

	var waiters []string
	for w := range p.waiters {
		waiters = append(waiters, w.String())
	}

	sort.Strings(waiters)

	return strings.Join(waiters, ",")
}

func (p *JobPool) Notify(builder string, blockNum uint64) {
	p.waitersMutex.Lock()
	defer p.waitersMutex.Unlock()

	zlog.Debug("pool: notification received", zap.String("builder", builder), zap.Uint64("block number", blockNum))

	for waiter := range p.waiters {
		waiter.Signal(builder, blockNum)
	}
}

func (p *JobPool) Add(ctx context.Context, reverseIdx int, job *Job, waiter Waiter) error {
	if p.readActive {
		return fmt.Errorf("cannot add to pool once reading has begun")
	}

	rw := &jobWaiter{
		ReverseIndex: reverseIdx,
		job:          job,
		Waiter:       waiter,
	}

	p.waitersMutex.Lock()
	p.totalJobs++
	p.waiters[rw.Waiter] = struct{}{}
	p.jobWaiters = append(p.jobWaiters, rw)
	p.waitersMutex.Unlock()

	return nil
}

func (p *JobPool) Start(ctx context.Context) {
	zlog.Debug("starting job pool")
	p.start.Do(func() {
		zlog.Debug("starting queue")
		StartQueue(ctx, p.queueSend, p.queueReceive)

		p.readActive = true

		wg := sync.WaitGroup{}

		zlog.Debug("adding to wait group", zap.Int("waiters", len(p.jobWaiters)))
		wg.Add(len(p.jobWaiters))

		go func() {
			wg.Wait()
			zlog.Debug("done waiting for wait group. closing job stream")
			close(p.readyJobStream)
		}()

		for _, rw := range p.jobWaiters {
			go func(item *jobWaiter) {
				defer wg.Done()

				select {
				case <-ctx.Done():
					return
				case <-item.Waiter.Wait(ctx):
					select {
					case <-ctx.Done():
						return
					case p.readyJobStream <- item:
						zlog.Debug("sent to ready job stream", zap.Stringer("job", item.job))
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
				close(p.queueSend) //finished sending items into queue once all jobs come through on the readyJobStream
			}()
			for {
				select {
				case <-ctx.Done():
					return
				case waiter, ok := <-p.readyJobStream:
					if !ok {
						return
					}

					// the number of stores a waiter was waiting for determines its priority in the queue.
					// higher number of stores => higher priority.
					// adjusted with the "reverse index"
					priority := waiter.Waiter.Size() + waiter.ReverseIndex

					zlog.Debug("sending job in queue",
						zap.Stringer("job", waiter.job),
						zap.Int("priority", priority),
					)

					select {
					case <-ctx.Done():
						return
					case p.queueSend <- &QueueItem{
						job:      waiter.job,
						Priority: priority,
					}:
						zlog.Debug("sent job in queue",
							zap.Stringer("job", waiter.job),
							zap.Int("priority", priority),
						)
					}

				}
			}
		}()
	})
}

func (p *JobPool) GetNext(ctx context.Context) (*Job, error) {
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

func (p *JobPool) Count() int {
	return p.totalJobs
}
