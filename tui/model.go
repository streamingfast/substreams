package tui

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func newModel(ui *TUI) model {
	return model{
		Modules:     updatedRanges{},
		ui:          ui,
		screenWidth: 120,
	}
}

type model struct {
	ui *TUI

	screenWidth int

	Modules           updatedRanges
	BarMode           bool
	DebugSetting      bool
	Updates           int
	UpdatedSecond     int64
	UpdatesPerSecond  int
	UpdatesThisSecond int

	Request   *pbsubstreams.Request
	Connected bool

	Failures    int
	LastFailure *pbsubstreams.ModuleProgress_Failed
	Reason      string
}
