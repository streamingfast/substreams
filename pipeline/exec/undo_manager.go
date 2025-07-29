package exec

import (
	"context"
	"errors"
	"math"
	"sync"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var GlobalUndoManager *UndoManager
var ErrBlockUndo = errors.New("block undone")
var MaxUndoneBlocksSize = 100

type watcher struct {
	cancel context.CancelCauseFunc
	id     uint64
}

// UndoManager gives cancelable contexts as subscriptions to specific block hashes. if that blocks gets an UNDO signal from a reorg, it will cancel that context.
type UndoManager struct {
	sync.Mutex
	activeSubscriptions  map[string][]watcher
	previousUndoneBlocks map[string]uint64
	idCounter            atomic.Uint64
}

func NewUndoManager() *UndoManager {
	return &UndoManager{
		activeSubscriptions:  make(map[string][]watcher),
		previousUndoneBlocks: make(map[string]uint64),
		idCounter:            atomic.Uint64{},
	}
}

func (u *UndoManager) ProcessBlock(blk *pbbstream.Block, obj any) error {
	steppable, ok := obj.(bstream.Stepable)
	if !ok {
		return nil
	}
	step := steppable.Step()
	switch {
	case step.Matches(bstream.StepNew):
		if u.Contains(blk.Id) {
			zlog.Warn("received 'NEW' block on a block that was previously state 'UNDO'. This may have disconnected some users for no good reason", zap.String("block_id", blk.Id), zap.Uint64("block_number", blk.Number))
			delete(u.previousUndoneBlocks, blk.Id)
		}
	case step.Matches(bstream.StepUndo):
		if tracer.Enabled() {
			zlog.Debug("undo manager process_block type UNDO", zap.Uint64("block_num", blk.Number))
		}

		u.Lock()
		u.notifyUndo(blk.Id)
		u.previousUndoneBlocks[blk.Id] = blk.Number
		if len(u.previousUndoneBlocks) > MaxUndoneBlocksSize {
			u.deleteOldestUndoneBlock()
		}
		u.Unlock()

	}

	return nil
}

func (u *UndoManager) deleteOldestUndoneBlock() {
	var oldestBlockID string
	var oldestBlockNumber uint64 = math.MaxUint64

	for id, number := range u.previousUndoneBlocks {
		if number < oldestBlockNumber {
			oldestBlockID = id
			oldestBlockNumber = number
		}
	}

	delete(u.previousUndoneBlocks, oldestBlockID)
}

func (u *UndoManager) Contains(id string) bool {
	u.Lock()
	_, ok := u.previousUndoneBlocks[id]
	u.Unlock()
	return ok
}

// Subscribe returns a context that will get canceled if the block with the given ID gets previousUndoneBlocks
// returns an unsubscribe function that must be called to prevent leaks
func (u *UndoManager) Subscribe(ctx context.Context, blockID string) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	watcher := watcher{
		cancel: cancel,
		id:     u.idCounter.Add(1),
	}
	if tracer.Enabled() {
		zlog.Debug("undo manager subscribed block", zap.String("block_id", blockID), zap.Uint64("watcher_id", watcher.id))
	}

	u.Lock()
	defer u.Unlock()

	_, alreadyUndone := u.previousUndoneBlocks[blockID]
	if alreadyUndone {
		cancel(ErrBlockUndo)
		return ctx, func() {}
	}
	u.activeSubscriptions[blockID] = append(u.activeSubscriptions[blockID], watcher)

	return ctx, func() {
		u.unsubscribe(blockID, watcher.id)
	}
}

func (u *UndoManager) unsubscribe(blockID string, watcherID uint64) {
	u.Lock()
	if tracer.Enabled() {
		zlog.Debug("undo manager unsubscribed block", zap.String("block_id", blockID), zap.Uint64("watcher_id", watcherID))
	}
	for i, w := range u.activeSubscriptions[blockID] {
		if w.id == watcherID {
			u.activeSubscriptions[blockID] = append(u.activeSubscriptions[blockID][:i], u.activeSubscriptions[blockID][i+1:]...)
			break
		}
	}
	if len(u.activeSubscriptions[blockID]) == 0 {
		delete(u.activeSubscriptions, blockID)
	}
	u.Unlock()
}

func (u *UndoManager) notifyUndo(blockID string) {
	for _, w := range u.activeSubscriptions[blockID] {
		if tracer.Enabled() {
			zlog.Debug("UNDO triggering cancel", zap.String("block_id", blockID), zap.Uint64("watcher_id", w.id))
		}
		w.cancel(ErrBlockUndo)
	}
}
