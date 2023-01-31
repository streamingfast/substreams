package output

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

type Output struct {
	common common.Common
	KeyMap KeyMap
}

func New(c common.Common) *Output {
	return &Output{
		common: c,
		KeyMap: DefaultKeyMap(),
	}
}

func (o *Output) Init() tea.Cmd { return nil }

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return o, nil
}

func (o *Output) View() string {
	return "output view"
}

func (o *Output) SetSize(width, height int) {
	o.common.SetSize(width, height)
}
