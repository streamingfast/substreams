package ranges

import tea "github.com/charmbracelet/bubbletea"

type Bar struct {
	name string
}

func New(name string) *Bar {
	return &Bar{name: name}
}

func (b Bar) Init() tea.Cmd { return nil }

func (b Bar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	//TODO implement me
	panic("implement me")
}

func (b Bar) View() string {
	// Return the bar
}
