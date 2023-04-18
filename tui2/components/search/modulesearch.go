package search

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

type ModuleSearchClearedMsg bool
type ApplyModuleSearchQueryMsg string

type ModuleSearch struct {
	input textinput.Model

	enabled bool
	Query   string
}

func NewModuleSearch() *ModuleSearch {
	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "/"
	input.CharLimit = 256
	input.Width = 80
	return &ModuleSearch{
		input: input,
	}
}

func (m *ModuleSearch) Init() tea.Cmd {
	return nil
}

func (m *ModuleSearch) View() string {
	if !m.input.Focused() {
		return fmt.Sprintf("/%s - ()", m.Query)
	} else {
		return m.input.View()
	}
}

func (m *ModuleSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.input.Focused() {
			switch msg.String() {

			// SEND NEW MESSSAGES
			case "enter":
				m.Query = m.input.Value()
				m.input.Blur()
				cmds = append(cmds, m.cancelModuleModal(), m.applyModuleSearchQuery(m.Query))
			case "backspace":
				if m.input.Value() == "" {
					cmds = append(cmds, m.cancelModuleModal(), m.clearModuleSearch)
				}
			}

			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)

		} else {
			switch msg.String() {
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *ModuleSearch) InitInput() tea.Cmd {
	m.input.Focus()
	m.input.SetValue("")
	return func() tea.Msg {
		return common.SetModuleModalUpdateFuncMsg(m.Update)
	}
}

func (m *ModuleSearch) cancelModuleModal() tea.Cmd {
	return func() tea.Msg {
		return common.SetModuleModalUpdateFuncMsg(nil)
	}
}

func (m *ModuleSearch) applyModuleSearchQuery(query string) tea.Cmd {
	return func() tea.Msg {
		return ApplyModuleSearchQueryMsg(query)
	}
}

func (m *ModuleSearch) clearModuleSearch() tea.Msg {
	return ModuleSearchClearedMsg(true)
}
