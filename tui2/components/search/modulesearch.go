package search

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

func (s *Search) cancelModuleModal() tea.Cmd {
	return func() tea.Msg {
		return common.SetModuleModalUpdateFuncMsg(nil)
	}
}
