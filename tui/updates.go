package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// Implement the tea.Model interface
func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m = m.withDefaults()

	switch msg {
	case Connecting:
		// Can be received again after the initial connection when the sinker retries a severed
		// stream. Everything the previous session reported is stale, including its trace ID and
		// the rate windows, which would otherwise show a discontinuity as a burst of progress.
		m.Connected = false
		m.TraceID = ""
		m.session = nil
		m.progress = nil
		m.globalRate.reset()
		m.moduleRates.reset()
	case Connected:
		m.Connected = true
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			m.ui.Interrupt()
			return m, tea.Quit
		}
		if msg.String() == "q" {
			return m, tea.Quit
		}

	case *pbsubstreamsrpc.SessionInit:
		// Receiving the session init message means the connection to the server is
		// established, the trace ID is only known at that point.
		m.Connected = true
		m.TraceID = msg.TraceId
		m.session = msg
		m.sessionAt = m.now()

	case *pbsubstreamsrpc.ModulesProgress:
		m.progress = msg

		at := m.now()
		m.globalRate.add(at, globalDone(msg))
		for _, stats := range msg.ModulesStats {
			m.moduleRates.add(at, stats.Name, stats.TotalProcessingTimeMs, stats.TotalProcessedBlockCount)
		}
	}

	return m, nil
}
