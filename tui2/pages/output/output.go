package output

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Output struct {
	common.Common
	KeyMap KeyMap

	updates int
}

func New(c common.Common) *Output {
	return &Output{
		Common: c,
		KeyMap: DefaultKeyMap(),
	}
}

func (o *Output) Init() tea.Cmd { return nil }

func (o *Output) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case stream.ResponseDataMsg:
		o.updates += 1
	}
	return o, nil
}

func (o *Output) View() string {
	return lipgloss.JoinVertical(0,
		"Output view",
		fmt.Sprintf("Data updates: %d", o.updates),
	)
}
