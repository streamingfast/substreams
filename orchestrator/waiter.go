package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Waiter interface {
	Wait(ctx context.Context) <-chan interface{}
	Signal(storeName string, blockNum uint64)
	Size() int
	BlockNumber() uint64
	String() string
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

func (wi *waiterItem) String() string {
	return fmt.Sprintf("waiter (store:%s) (block:%d)", wi.StoreName, wi.BlockNum)
}

type waiterItems []*waiterItem

func (wis waiterItems) String() string {
	var wislice []string
	for _, wi := range wis {
		wislice = append(wislice, wi.String())
	}

	return strings.Join(wislice, ",")
}

type BlockWaiter struct {
	items []*waiterItem

	setup sync.Once

	closeOnce sync.Once
	done      chan interface{}

	name     string
	blockNum uint64
}

func NewWaiter(name string, blockNum uint64, stores ...*pbsubstreams.Module) *BlockWaiter {
	var items []*waiterItem

	for _, store := range stores {
		if blockNum <= store.InitialBlock {
			continue
		}

		items = append(items, &waiterItem{
			StoreName: store.Name,
			BlockNum:  blockNum,
			waitChan:  make(chan interface{}),
		})
	}

	return &BlockWaiter{
		items: items,

		name:     name,
		blockNum: blockNum,

		done: make(chan interface{}),
	}
}

func (w *BlockWaiter) Wait(ctx context.Context) <-chan interface{} {
	w.setup.Do(func() {
		if len(w.items) == 0 {
			// nothing to wait for.
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(len(w.items))

		go func() {
			wg.Wait()
			zlog.Debug("block waiter done waiting", zap.String("module", w.name), zap.Uint64("block_num", w.blockNum))
			w.closeOnce.Do(func() {
				close(w.done)
			})
		}()

		for _, item := range w.items {
			go func(waiterItem *waiterItem) {
				defer wg.Done()

				select {
				case <-waiterItem.Wait():
					return
				case <-ctx.Done():
					return
				}

			}(item)
		}
	})

	return w.done
}

func (w *BlockWaiter) Signal(storeName string, blockNum uint64) {
	if len(w.items) == 0 {
		zlog.Debug("block waiter done waiting (nothing to wait for)", zap.String("module", w.name), zap.Uint64("block_num", w.blockNum))
		w.closeOnce.Do(func() {
			close(w.done)
		})
		return
	}

	if storeName == "" && blockNum == 0 { //ignore blank signal here
		return
	}

	for _, waiterItem := range w.items {
		if waiterItem.StoreName != storeName {
			continue
		}

		if waiterItem.BlockNum > blockNum {
			continue
		}

		waiterItem.Close()
	}
}

func (w *BlockWaiter) Size() int {
	return len(w.items)
}

func (w *BlockWaiter) BlockNumber() uint64 {
	return w.blockNum
}

func (w *BlockWaiter) String() string {
	if w.items == nil {
		return fmt.Sprintf("[%s] O(%d)", "nil", w.Size())
	}

	var wis []string
	for _, wi := range w.items {
		wis = append(wis, wi.String())
	}

	return fmt.Sprintf("[%s] O(%d)", strings.Join(wis, ","), w.Size())
}
