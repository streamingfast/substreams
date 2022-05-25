package decode

import "fmt"

type ModuleName string

type ModuleProgressBar struct {
	bars map[ModuleName]*Bar
}

type Bar struct {
	percent uint64 // progress percentage
	start   uint64 // starting point for progress
	cur     uint64 // current progress
	total   uint64 // total value for progress
	rate    string // the actual progress bar to be printed
	graph   string // the fill value for progress bar
}

func (bar *Bar) NewOption(cur, total uint64) {
	bar.cur = cur
	bar.total = total
	bar.graph = "#"
	bar.percent = bar.getPercent()
	for i := 0; i < int(bar.percent); i += 2 {
		bar.rate += bar.graph // initial progress position
	}
}

func (bar *Bar) NewOptionStartingPoint(start uint64) {
	bar.start = start
}

func (bar *Bar) Play(cur uint64) {
	bar.cur = cur
	last := bar.percent
	bar.percent = bar.getPercent()
	if bar.percent != last && bar.percent%2 == 0 {
		bar.rate += bar.graph
	}
	fmt.Printf("\r[%-50s]%3d%% %8d/%d", bar.rate, bar.percent, bar.cur, bar.total)
}

func (bar *Bar) Finish() {
	fmt.Println()
}

func (bar *Bar) getPercent() uint64 {
	return uint64(float32(bar.cur) / float32(bar.total) * 100)
}
