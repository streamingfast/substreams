package orchestrator

import (
	"context"
	"sync"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Waiter interface {
	Wait(ctx context.Context) <-chan interface{}
	Signal(storeName string, blockNum uint64)
	Order() int
}

type waiterItem struct {
	StoreName string
	BlockNum  uint64 // This job requires waiting on this particular block number to be unblocked.

	closeOnce sync.Once
	waitChan  chan interface{}
}

func (wi *waiterItem) Close() {
	wi.closeOnce.Do(func() {
		close(wi.waitChan)
	})
}

func (wi *waiterItem) Wait() <-chan interface{} {
	return wi.waitChan
}

type BlockWaiter struct {
	items []*waiterItem

	storageState *StorageState
	setup        sync.Once
	done         chan interface{}
}

func NewWaiter(blockNum uint64, storageState *StorageState, stores ...*pbsubstreams.Module) *BlockWaiter {
	var items []*waiterItem

	for _, store := range stores {
		items = append(items, &waiterItem{
			StoreName: store.Name,
			BlockNum:  blockNum,
			waitChan:  make(chan interface{}),
		})
	}

	return &BlockWaiter{
		items:        items,
		storageState: storageState,
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

				if waiter.BlockNum <= w.storageState.lastBlocks[waiter.StoreName] {
					return //store has already saved up to or past the desired block.
				}

				select {
				case <-waiter.Wait():
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
		if waiter.StoreName != storeName {
			continue
		}

		if waiter.BlockNum > blockNum {
			continue
		}

		waiter.Close()
	}
}

func (w *BlockWaiter) Order() int {
	return len(w.items)
}
