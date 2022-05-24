package orchestrator

import (
	"context"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"sync"
)

type Waiter interface {
	Wait(ctx context.Context) <-chan interface{}
	Signal(storeName string, blockNum uint64)
}

type waiterItem struct {
	storeName string
	blockNum  uint64

	waitChan chan interface{}
}

type BlockWaiter struct {
	items []*waiterItem

	setup sync.Once
	done  chan interface{}
}

func NewWaiter(blockRange *block.Range, stores ...*pbsubstreams.Module) *BlockWaiter {
	var items []*waiterItem

	for _, store := range stores {
		items = append(items, &waiterItem{
			storeName: store.Name,
			blockNum:  blockRange.StartBlock,
			waitChan:  make(chan interface{}),
		})
	}

	return &BlockWaiter{
		items: items,
	}
}

func (w *BlockWaiter) Wait(ctx context.Context) <-chan interface{} {
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
			go func(waiter *waiterItem) {
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

func (w *BlockWaiter) Signal(storeName string, blockNum uint64) {
	for _, waiter := range w.items {
		if waiter.storeName != storeName || waiter.blockNum > blockNum {
			continue
		}

		close(waiter.waitChan)
	}
}
