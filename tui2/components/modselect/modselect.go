package modselect

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
)

type ModuleSelectedMsg string

// A vertical bar that allows you to select a module that has been seen
type ModSelect struct {
	common.Common
	Modules  []string
	Selected int
}

func New(c common.Common) *ModSelect {
	return &ModSelect{
		Common: c,
	}
}

func (m *ModSelect) Init() tea.Cmd { return nil }

func (m *ModSelect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if len(m.Modules) == 0 {
			break
		}
		switch msg.String() {
		case "u":
			m.Selected = (m.Selected - 1 + len(m.Modules)) % len(m.Modules)
			cmds = append(cmds, m.dispatchModuleSelected)
		case "i":
			m.Selected = (m.Selected + 1) % len(m.Modules)
			cmds = append(cmds, m.dispatchModuleSelected)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m *ModSelect) AddModule(modName string) {
	m.Modules = append(m.Modules, modName)
}

func (m *ModSelect) dispatchModuleSelected() tea.Msg {
	return ModuleSelectedMsg(m.Modules[m.Selected])
}
func (m *ModSelect) View() string {
	if len(m.Modules) == 0 {
		return ""
	}
	var mods []string
	for idx, mod := range m.Modules {
		if idx == m.Selected {
			mods = append(mods, Styles.SelectedModule.Render(mod))
		} else {
			mods = append(mods, Styles.UnselectedModule.Render(mod))
		}
	}
	return Styles.Box.MaxWidth(m.Width).Render(strings.Join(mods, ""))
}

var Styles = struct {
	Box              lipgloss.Style
	SelectedModule   lipgloss.Style
	UnselectedModule lipgloss.Style
}{
	Box:              lipgloss.NewStyle().Margin(1, 1),
	SelectedModule:   lipgloss.NewStyle().Margin(0, 1).Foreground(lipgloss.Color("12")).Bold(true),
	UnselectedModule: lipgloss.NewStyle().Margin(0, 1),
}
