package tui

import (
	"time"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func newModel(ui *TUI) model {
	m := model{ui: ui}
	return m.withDefaults()
}

type model struct {
	ui *TUI

	// now is injectable so that windowed rates, ETAs and job ages are deterministic in tests.
	now func() time.Time

	Connected bool
	TraceID   string

	// Width is 0 until the first tea.WindowSizeMsg is received.
	Width int

	session   *pbsubstreamsrpc.SessionInit
	sessionAt time.Time
	progress  *pbsubstreamsrpc.ModulesProgress

	globalRate  *rateWindow
	moduleRates *moduleWindow

	Failures    int
	LastFailure *pbsubstreamsrpc.Error
}

// withDefaults fills in what the model cannot work without. Tests build a model as a bare
// literal, so this cannot live in the constructor alone.
func (m model) withDefaults() model {
	if m.now == nil {
		m.now = time.Now
	}
	if m.globalRate == nil {
		m.globalRate = newRateWindow(rateWindowDuration)
	}
	if m.moduleRates == nil {
		m.moduleRates = newModuleWindow(rateWindowDuration)
	}
	return m
}
