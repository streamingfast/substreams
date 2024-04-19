package tui

import (
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func newModel(ui *TUI) model {
	return model{
		StagesProgress: updatedRanges{},
		ui:             ui,
	}
}

type model struct {
	ui *TUI

	StagesProgress updatedRanges
	StagesModules  []string
	SlowJobs       []string
	SlowModules    []string

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
	LastFailure *pbsubstreamsrpc.Error
	Reason      string

	TraceID string
}
