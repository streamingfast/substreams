package dataentry

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/keymap"
)

type DataEntry struct {
	common.Common
	common.SimpleHelp
	Input *huh.Input
	Form  *huh.Form
}

func New(c common.Common, field string, validation func(input string) error) *DataEntry {
	input := huh.NewInput().
		Key(field)
	if validation != nil {
		input.Validate(validation)
	}
	input.WithAccessible(true)

	form := huh.NewForm(huh.NewGroup(input).WithShowErrors(true)).WithTheme(huh.ThemeCharm())

	return &DataEntry{
		Common:     c,
		SimpleHelp: common.NewSimpleHelp(keymap.EscapeModal, keymap.EnterAcceptValue),
		Input:      input,
		Form:       form,
	}
}

func (m *DataEntry) SetValue(val string) {
	m.Input.Value(&val)
}

func (m *DataEntry) Init() tea.Cmd {
	return m.Form.Init()
}

func (m *DataEntry) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	//var cmds []tea.Cmd
	var cmd tea.Cmd
	model, cmd := m.Form.Update(msg)
	m.Form = model.(*huh.Form)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, common.CancelModalCmd()
		case "enter":
			if m.Input.Error() == nil {
				val := m.Input.GetValue().(string)
				return m, tea.Batch(common.SetRequestValueCmd(m.Input.GetKey(), val), common.CancelModalCmd())
			}
		}
	}
	return m, cmd
}

func (m *DataEntry) View() string {
	return m.Form.View()
}

func (m *DataEntry) SetSize(w, h int) {
	m.Common.SetSize(w, h)
	m.Form.WithWidth(w).WithHeight(6)
}

func (m *DataEntry) GetHeight() int {
	return lipgloss.Height(m.View())
}
