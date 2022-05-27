package decode

import (
	"fmt"
)

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
	cur     uint64 // current progress
}

func (bar *Bar) NewOption(cur, blockRangeStartBlock, blockRangeEndBlock uint64) {
	bar.Initialized = true
	bar.Total = blockRangeEndBlock - blockRangeStartBlock
	bar.BlockRangeStartBlock = blockRangeStartBlock
	bar.BlockRangeEndBlock = blockRangeEndBlock

	bar.cur = cur
	bar.percent = bar.getPercent()
}

func (bar *Bar) Play(cur uint64, moduleName string) {
	bar.cur = cur
	bar.percent = bar.getPercent()

	fmt.Printf("\r Executing: %s to catch up for blocks %d - %d -- %3d%% %8d/%d",
		moduleName,
		bar.BlockRangeStartBlock,
		bar.BlockRangeEndBlock,
		bar.percent,
		bar.cur,
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
	return uint64(float32(bar.cur) / float32(bar.Total) * 100)
}
