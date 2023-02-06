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
	var firstPart, lastPart, tmp []string
	var activeModule string
	for idx, mod := range m.Modules {
		if idx == m.Selected {
			activeModule = mod
			firstPart = tmp[:]
			tmp = nil
		} else {
			tmp = append(tmp, mod)
		}
	}
	lastPart = tmp

	sidePartsWidth := (m.Width-len(activeModule)-2)/2 - 3
	leftModules := strings.Join(firstPart, "  ")
	leftWidth := len(leftModules)
	if leftWidth > sidePartsWidth {
		leftModules = "..." + leftModules[leftWidth-sidePartsWidth:]
	}
	rightModules := strings.Join(lastPart, "  ")
	rightWidth := len(rightModules)
	if rightWidth > sidePartsWidth {
		rightModules = rightModules[:sidePartsWidth] + "..."
	}

	alignRight := lipgloss.NewStyle().Width(sidePartsWidth + 4).Align(lipgloss.Right)
	alignLeft := lipgloss.NewStyle().Width(sidePartsWidth + 4).Align(lipgloss.Left)
	return Styles.Box.MaxWidth(m.Width).Render(
		lipgloss.JoinHorizontal(0.5,
			alignRight.Render(leftModules),
			Styles.SelectedModule.Render(activeModule),
			alignLeft.Render(rightModules),
		))
}

var Styles = struct {
	Box              lipgloss.Style
	SelectedModule   lipgloss.Style
	UnselectedModule lipgloss.Style
}{
	Box:            lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true),
	SelectedModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("12")).Bold(true),
}
