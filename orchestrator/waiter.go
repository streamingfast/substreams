package orchestrator

import (
	"context"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"sync"
)

type Notifier interface {
	Notify(builder string, blockNum uint64)
}

type WaiterItem struct {
	builder  string
	blockNum uint64

	waitChan chan interface{}
}

type Waiter struct {
	items []*WaiterItem

	setup sync.Once
	done  chan interface{}
}

func NewWaiter(blockRange *block.Range, stores ...*pbsubstreams.Module) *Waiter {
	var items []*WaiterItem

	for _, store := range stores {
		items = append(items, &WaiterItem{
			builder:  store.Name,
			blockNum: blockRange.StartBlock,
			waitChan: make(chan interface{}),
		})
	}

	return &Waiter{
		items: items,
	}
}

func (w *Waiter) Done(ctx context.Context) <-chan interface{} {
	w.setup.Do(func() {
		w.done = make(chan interface{})

		if len(w.items) == 0 {
			close(w.done)
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(len(w.items))

		go func() {
			wg.Wait()
			close(w.done)
		}()

		for _, waiter := range w.items {
			go func(waiter *WaiterItem) {
				defer wg.Done()

				select {
				case <-waiter.waitChan:
					return
				case <-ctx.Done():
					return
				}

			}(waiter)
		}
	})

	return w.done
}

func (w *Waiter) Signal(builder string, blockNum uint64) {
	for _, waiter := range w.items {
		if waiter.builder != builder || waiter.blockNum > blockNum {
			continue
		}

		close(waiter.waitChan)
	}
}
