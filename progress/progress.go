package progress

import (
	"context"
	"fmt"
	"github.com/streamingfast/logging"
	"time"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
)

var zlog, _ = logging.PackageLogger("progress", "github.com/streamingfast/substreams/progress")

type Tracker struct {
	startAt                  time.Time
	processedBlockLastSecond int
	processedBlockCount      int
	blockSecond              int
	LastBlock                uint64
	timeSpentInStreamFuncs   time.Duration
}

func NewProgressTracker() *Tracker {
	return &Tracker{}
}

func (t *Tracker) StartTracking(ctx context.Context) {
	t.startAt = time.Now()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				t.blockSecond = t.processedBlockCount - t.processedBlockLastSecond
				t.processedBlockLastSecond = t.processedBlockCount
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				t.log()
			}
		}
	}()
}

func (t *Tracker) BlockProcessed(block *bstream.Block, delta time.Duration) {
	t.processedBlockCount += 1
	t.LastBlock = block.Num()
	t.timeSpentInStreamFuncs += delta
}

func (t *Tracker) log() {
	zlog.Info("progress",
		zap.Uint64("last_block", t.LastBlock),
		zap.Int("total_processed_block", t.processedBlockCount),
		zap.Int("block_second", t.blockSecond),
		zap.Duration("stream_func_deltas", t.timeSpentInStreamFuncs))
	t.timeSpentInStreamFuncs = 0
}

type ModuleName string

type ModuleProgressBar struct {
	Bars map[ModuleName]*Bar
}

type Bar struct {
	Initialized          bool
	Total                uint64 // Total value for progress
	BlockRangeStartBlock uint64 // BlockRangeStartBlock value for start block for current request
	BlockRangeEndBlock   uint64 // BlockRangeEndBlock value for end block for current request

	percent uint64 // progress percentage
	start   uint64 // starting point for progress
	Cur     uint64 // current progress
}

func (bar *Bar) NewOption(cur, blockRangeStartBlock, blockRangeEndBlock uint64) {
	bar.Initialized = true
	bar.Total = blockRangeEndBlock - blockRangeStartBlock
	bar.BlockRangeStartBlock = blockRangeStartBlock
	bar.BlockRangeEndBlock = blockRangeEndBlock

	bar.Cur = cur
	bar.percent = bar.getPercent()
}

func (bar *Bar) Play(cur uint64, moduleName string) {
	bar.Cur = cur
	bar.percent = bar.getPercent()

	fmt.Printf("\r Executing: %s to catch up for blocks %d - %d -- %3d%% %8d/%d",
		moduleName,
		bar.BlockRangeStartBlock,
		bar.BlockRangeEndBlock,
		bar.percent,
		bar.Cur,
		bar.Total)
}

func (bar *Bar) Finish() {
	fmt.Println()
	bar.Initialized = false
	bar.percent = 0
	bar.start = 0
	bar.Total = 0
	bar.BlockRangeStartBlock = 0
	bar.BlockRangeEndBlock = 0
}

func (bar *Bar) getPercent() uint64 {
	return uint64(float32(bar.Cur) / float32(bar.Total) * 100)
}
