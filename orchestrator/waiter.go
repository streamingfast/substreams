package orchestrator

import (
	"context"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"sync"
)

type Waiter interface {
	Wait(ctx context.Context) <-chan interface{}
	Signal(storeName string, blockNum uint64)
	Order() int
}

type waiterItem struct {
	storeName string
	blockNum  uint64

	waitChan chan interface{}
}

type BlockWaiter struct {
	items []*waiterItem

	lastSavedBlockMap map[string]uint64
	setup             sync.Once
	done              chan interface{}
}

func NewWaiter(blockNum uint64, lastSavedBlockMap map[string]uint64, stores ...*pbsubstreams.Module) *BlockWaiter {
	var items []*waiterItem

	for _, store := range stores {
		items = append(items, &waiterItem{
			storeName: store.Name,
			blockNum:  blockNum,
			waitChan:  make(chan interface{}),
		})
	}

	return &BlockWaiter{
		items:             items,
		lastSavedBlockMap: lastSavedBlockMap,
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

				if waiter.blockNum <= w.lastSavedBlockMap[waiter.storeName] {
					return //store has already saved up to or past the desired block.
				}

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

func (w *BlockWaiter) Order() int {
	return len(w.items)
}
