package decode

import "fmt"

type ModuleName string

type ModuleProgressBar struct {
	Bars map[ModuleName]*Bar
}

type Bar struct {
	Initialized bool
	percent     uint64 // progress percentage
	start       uint64 // starting point for progress
	cur         uint64 // current progress
	Total       uint64 // Total value for progress
	rate        string // the actual progress bar to be printed
	graph       string // the fill value for progress bar
}

func (bar *Bar) NewOption(cur, total uint64) {
	bar.cur = cur
	bar.Total = total
	bar.graph = "#"
	bar.percent = bar.getPercent()
	for i := 0; i < int(bar.percent); i += 2 {
		bar.rate += bar.graph // initial progress position
	}
}

func (bar *Bar) Play(cur uint64, moduleName string) {
	bar.cur = cur
	last := bar.percent
	bar.percent = bar.getPercent()
	if bar.percent != last && bar.percent%2 == 0 {
		bar.rate += bar.graph
	}
	fmt.Printf("\r%s [%-50s]%3d%% %8d/%d", moduleName, bar.rate, bar.percent, bar.cur, bar.Total)
}

func (bar *Bar) Finish() {
	fmt.Println()
	bar.Initialized = false
	bar.percent = 0
	bar.start = 0
	bar.Total = 0
	bar.rate = ""
}

func (bar *Bar) getPercent() uint64 {
	return uint64(float32(bar.cur) / float32(bar.Total) * 100)
}
