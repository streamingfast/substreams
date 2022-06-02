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
		// case Quit:
		// 	return nil, tea.Quit
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			fmt.Println("Quitting...")
			m.ui.Cancel()
			// TODO: trigger downstream shutdown of the blocks processing
			return m, tea.Quit
		}
		switch msg.String() {
		case "enter":
			fmt.Println("")
		case "m":
			m.BarMode = !m.BarMode
			m.DebugSetting = !m.DebugSetting
		case "q":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.screenWidth = msg.Width - 45
	case *pbsubstreams.Request:
		m.Request = msg
		return m, nil
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
			return m, nil
		}
	default:
	}

	return m, nil
}
