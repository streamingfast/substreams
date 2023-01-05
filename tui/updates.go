package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// Implement the tea.Model interface
func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg {
	case Connecting:
		m.Connected = false
	case Connected:
		m.Connected = true
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width < 45 {
			m.BarSize = 4
		} else {
			m.BarSize = uint64(msg.Width) - 45
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			m.ui.Cancel()
			fmt.Println("Interrupted UI")
			return m, tea.Quit
		}
		switch msg.String() {
		case "m":
			m.BarMode = !m.BarMode
			return m, nil
		case "q":
			return m, tea.Quit
		}
	case *pbsubstreams.Request:
		m.Request = msg
		// It's ok to use `StartBlockNum` directly instead of effective start block (start block
		// of cursor if present, `StartBlockNum` otherwise) because this is only used in backprocessing
		// `barmode` which is effective only when no cursor has been passed yet.
		m.BackprocessingCompleteAtBlock = uint64(m.Request.StartBlockNum)
		return m, nil
	case *pbsubstreams.Response_Session:
		m.TraceID = msg.Session.TraceId

	case *pbsubstreams.ModuleProgress:
		m.Updates += 1
		thisSec := time.Now().Unix()
		if m.UpdatedSecond != thisSec {
			m.UpdatesPerSecond = m.UpdatesThisSecond
			m.UpdatesThisSecond = 0
			m.UpdatedSecond = thisSec
		}
		m.UpdatesThisSecond += 1

		switch progMsg := msg.Type.(type) {
		case *pbsubstreams.ModuleProgress_ProcessedRanges:
			newModules := updatedRanges{}
			for k, v := range m.Modules {
				newModules[k] = v
			}

			for _, v := range progMsg.ProcessedRanges.ProcessedRanges {
				newModules[msg.Name] = mergeRangeLists(newModules[msg.Name], &blockRange{
					Start: v.StartBlock,
					End:   v.EndBlock,
				})
			}

			m.Modules = newModules
		case *pbsubstreams.ModuleProgress_InitialState_:
		case *pbsubstreams.ModuleProgress_ProcessedBytes_:
		case *pbsubstreams.ModuleProgress_Failed_:
			m.Failures += 1
			if progMsg.Failed.Reason != "" {
				m.Reason = fmt.Sprintf("Reason: %s, logs: %s, truncated: %v", progMsg.Failed.Reason, progMsg.Failed.Logs, progMsg.Failed.LogsTruncated)
			}
			m.LastFailure = progMsg.Failed
			m.ui.Cancel()
			return m, nil
		}
	default:
	}

	return m, nil
}
