package pipeline

import (
	"context"
	"time"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
)

type progressTracker struct {
	startAt                  time.Time
	processedBlockLastSecond int
	processedBlockCount      int
	blockSecond              int
	lastBlock                uint64
	timeSpentInStreamFuncs   time.Duration
}

func newProgressTracker() *progressTracker {
	return &progressTracker{}
}

func (p *progressTracker) startTracking(ctx context.Context) {
	p.startAt = time.Now()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				p.blockSecond = p.processedBlockCount - p.processedBlockLastSecond
				p.processedBlockLastSecond = p.processedBlockCount
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				p.log()
			}
		}
	}()
}

func (p *progressTracker) blockProcessed(block *bstream.Block, delta time.Duration) {
	p.processedBlockCount += 1
	p.lastBlock = block.Num()
	p.timeSpentInStreamFuncs += delta
}

func (p *progressTracker) log() {
	zlog.Info("progress",
		zap.Uint64("last_block", p.lastBlock),
		zap.Int("total_processed_block", p.processedBlockCount),
		zap.Int("block_second", p.blockSecond),
		zap.Duration("stream_func_deltas", p.timeSpentInStreamFuncs))
	p.timeSpentInStreamFuncs = 0
}
