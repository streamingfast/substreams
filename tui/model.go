package tui

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

func newModel(ui *TUI) model {
	return model{
		Modules: updatedRanges{},
		ui:      ui,
	}
}

type model struct {
	ui *TUI

	Modules      updatedRanges
	DebugSetting bool
	Updates      int

	Request   *pbsubstreams.Request
	Connected bool

	Failures int
	Reason   string
	Clock    *pbsubstreams.Clock
}
