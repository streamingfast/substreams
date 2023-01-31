package progress

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

type Progress struct {
	common common.Common
	KeyMap KeyMap
}

func New(c common.Common) *Progress {
	return &Progress{
		common: c,
		KeyMap: DefaultKeyMap(),
	}
}

func (p *Progress) Init() tea.Cmd { return nil }

func (p *Progress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return p, nil
}

func (p *Progress) View() string {
	return "progress view"
}

func (p *Progress) SetSize(width, height int) {
	p.common.SetSize(width, height)
}
