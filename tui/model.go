package tui

import (
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func newModel(ui *TUI) model {
	return model{
		Modules: updatedRanges{},
		ui:      ui,
	}
}

type model struct {
	ui *TUI

	Modules updatedRanges
	BarMode bool
	BarSize uint64

	Updates           int
	UpdatedSecond     int64
	UpdatesPerSecond  int
	UpdatesThisSecond int

	Request                       *pbsubstreamsrpc.Request
	BackprocessingCompleteAtBlock uint64
	Connected                     bool

	Failures    int
	LastFailure error //*pbsubstreamsrpc.ModuleProgress_Failed
	Reason      string

	TraceID string
}
