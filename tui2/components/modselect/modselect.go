package modselect

import (
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
)

// A vertical bar that allows you to select a module that has been seen
type ModSelect struct {
	common.Common
	seen map[string]bool

	Modules      []string
	ModulesIndex map[string]int

	Selected int

	moduleGraph *manifest.ModuleGraph
}

func New(c common.Common, graph *manifest.ModuleGraph) *ModSelect {

	return &ModSelect{
		seen:         map[string]bool{},
		ModulesIndex: map[string]int{},
		Common:       c,
		moduleGraph:  graph,
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
			newSelection := (m.Selected - 1 + len(m.Modules)) % len(m.Modules)
			cmds = append(cmds, common.EmitModuleSelectedMsg(m.Modules[newSelection]))
		case "i":
			newSelection := (m.Selected + 1) % len(m.Modules)
			cmds = append(cmds, common.EmitModuleSelectedMsg(m.Modules[newSelection]))
		}
	case common.ModuleSelectedMsg:
		m.Selected = m.ModulesIndex[string(msg)]
		log.Println("Module selected dude", msg, m.Selected)
	}
	return m, tea.Batch(cmds...)
}

func (m *ModSelect) AddModule(modName string) bool {
	if !m.seen[modName] {
		m.Modules = append(m.Modules, modName)
		m.ModulesIndex[modName] = len(m.Modules) - 1
		m.seen[modName] = true
		//
		//// sort the modules
		//
		//sorted, _ := m.moduleGraph.TopologicalSortKnownModules(m.seen)
		//
		//newModules := make([]string, 0, len(m.Modules))
		//var newSelected int
		//for idx, mod := range sorted {
		//	newModules = append(newModules, mod.Name)
		//	if mod.Name == m.Modules[m.Selected] {
		//		newSelected = idx
		//	}
		//}
		//
		//m.Modules = newModules
		//m.Selected = newSelected
		return true
	}
	return false
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
	return m.Styles.ModSelect.Box.MaxWidth(m.Width).Render(
		lipgloss.JoinHorizontal(0.5,
			alignRight.Render(leftModules),
			m.Styles.ModSelect.SelectedModule.Render(activeModule),
			alignLeft.Render(rightModules),
		),
	)
}
