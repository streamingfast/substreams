package modsearch

import (
	"log"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/tui2/common"
)

type DisableModuleSearch bool
type ApplyModuleSearchQueryMsg string
type UpdateModuleSearchQueryMsg string

type ModuleSearch struct {
	common.Common
	input textinput.Model

	matchesView viewport.Model

	seenModules      []string
	matchingModules  []string
	highlightedIndex int
}

func New(c common.Common) *ModuleSearch {
	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "/"
	input.CharLimit = 256
	input.Width = 80
	return &ModuleSearch{
		Common:      c,
		input:       input,
		matchesView: viewport.New(24, 79),
	}
}

func (m *ModuleSearch) Init() tea.Cmd {
	return nil
}

func (m *ModuleSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
msgSwitch:
	switch msg := msg.(type) {
	case common.UpdateSeenModulesMsg:
		log.Println("Updated seen modules!", msg)
		m.seenModules = msg
	case tea.KeyMsg:
		if m.input.Focused() {
			switch msg.String() {
			case "enter":
				m.input.Blur()
				cmds = append(cmds, m.cancelModuleModal(), m.emitDisableMsg, common.EmitModuleSelectedMsg(m.selectedModule()))
				break msgSwitch
			case "backspace":
				if m.input.Value() == "" {
					cmds = append(cmds, m.cancelModuleModal(), m.emitDisableMsg)
				}
			case "up":
				if m.highlightedIndex != 0 {
					m.highlightedIndex--
				}
			case "down":
				if m.highlightedIndex != len(m.matchingModules)-1 {
					m.highlightedIndex++
				}
			default:
				m.highlightedIndex = 0
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
			m.updateViewport()
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *ModuleSearch) selectedModule() string {
	if m.highlightedIndex >= len(m.matchingModules) {
		return ""
	}
	return m.matchingModules[m.highlightedIndex]
}

func (m *ModuleSearch) updateViewport() {
	m.matchesView.SetContent(m.renderMatches(m.input.Value()))
}

func (m *ModuleSearch) InitInput() tea.Cmd {
	m.input.Focus()
	m.input.SetValue("")
	m.updateViewport()
	m.highlightedIndex = 0
	return func() tea.Msg {
		return common.SetModalUpdateFuncMsg(m.Update)
	}
}

func (m *ModuleSearch) renderMatches(query string) string {
	var matchingMods []string
	for _, mod := range m.seenModules {
		if containsPortions(mod, query) {
			matchingMods = append(matchingMods, mod)
		}
	}
	if len(matchingMods) == 0 {
		m.highlightedIndex = 0
		return ""
	}

	if m.highlightedIndex >= len(matchingMods) {
		m.highlightedIndex = len(matchingMods) - 1
	}

	m.matchingModules = matchingMods

	var rows []string
	maxHeight := m.matchesView.Height - 2
	for idx, modName := range matchingMods {
		if idx >= maxHeight {
			break
		}
		if idx == m.highlightedIndex {
			rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(modName))
		} else {
			rows = append(rows, modName)
		}
	}

	out := lipgloss.JoinVertical(0, rows...)
	return out
}

func containsPortions(modName, query string) bool {
	queryIndex := 0
	for _, r := range modName {
		if queryIndex < len(query) && r == rune(query[queryIndex]) {
			queryIndex++
		}
	}
	return queryIndex == len(query)
}

func (m *ModuleSearch) View() string {
	return lipgloss.JoinVertical(0,
		m.input.View(),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(m.Width).Render(m.matchesView.View()),
	)
}

func (m *ModuleSearch) SetSize(w, h int) {
	m.Common.SetSize(w, h)
	m.matchesView.Width = w
	m.matchesView.Height = h - 3
}

func (m *ModuleSearch) cancelModuleModal() tea.Cmd {
	return func() tea.Msg {
		return common.SetModalUpdateFuncMsg(nil)
	}
}

func (m *ModuleSearch) emitDisableMsg() tea.Msg {
	return DisableModuleSearch(true)
}
