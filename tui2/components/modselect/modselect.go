package modselect

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/yourbasic/graph"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
)

type SelectNextModuleMsg string // TODO: use the same pattern as `blockselect`
type SelectPreviousModuleMsg string
type ModuleSelectedMsg string

// A vertical bar that allows you to select a module that has been seen
type ModSelect struct {
	common.Common

	Seen    map[string]bool
	Modules []string

	columnsCache [][]int

	Selected            int
	SelectedColumn      int
	SelectedColumnIndex int

	Highlighted            int
	HighlightedColumn      int
	HighlightedColumnIndex int

	moduleGraph *manifest.ModuleGraph
}

func New(c common.Common, manifestPath string) *ModSelect {
	graph := manifest.MustNewModuleGraph(manifest.NewReader(manifestPath).MustRead().Modules.Modules)
	modules := graph.Modules()

	return &ModSelect{
		Common:  c,
		Seen:    map[string]bool{},
		Modules: modules,

		moduleGraph: graph,
	}
}

func newTestModSelect(modules []*pbsubstreams.Module) *ModSelect {
	graph := manifest.MustNewModuleGraph(modules)

	return &ModSelect{
		Common:  common.Common{},
		Seen:    map[string]bool{},
		Modules: graph.Modules(),

		moduleGraph: graph,
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
	if !m.Seen[modName] {
		m.Modules = append(m.Modules, modName)
		m.Seen[modName] = true
	}
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
		),
	)
}

var Styles = struct {
	Box               lipgloss.Style
	SelectedModule    lipgloss.Style
	HighlightedModule lipgloss.Style
	UnselectedModule  lipgloss.Style
	UnavailableModule lipgloss.Style
}{
	Box:               lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true),
	SelectedModule:    lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("12")).Bold(true),
	HighlightedModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("21")).Bold(true),
	UnavailableModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("8")).Bold(false),
	UnselectedModule:  lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.Color("0")).Bold(false),
}

func (m *ModSelect) GetColumns(targetNode int) ([][]int, error) {
	if m.columnsCache != nil {
		return m.columnsCache, nil
	}

	g := m.moduleGraph
	_, distances := graph.ShortestPaths(g, targetNode)

	alreadyAdded := map[string]bool{}
	distanceMap := map[int64][]int{}

	for i, d := range distances {
		if d < 0 {
			continue
		}

		module := g.ModuleNameFromIndex(i)
		if _, ok := alreadyAdded[module]; ok {
			continue
		}

		if distanceMap[d] == nil {
			distanceMap[d] = []int{}
		}
		distanceMap[d] = append(distanceMap[d], i)
	}

	var distanceKeys []int64
	for k := range distanceMap {
		distanceKeys = append(distanceKeys, k)
	}
	sort.Slice(distanceKeys, func(i, j int) bool {
		return distanceKeys[i] < distanceKeys[j]
	})

	res := make([][]int, len(distanceKeys))

	for i, d := range distanceKeys {
		res[i] = distanceMap[d]
	}

	m.columnsCache = res
	return res, nil
}

func (m *ModSelect) GetRenderedColumns(targetNode int) ([][]string, error) {
	columns, err := m.GetColumns(targetNode)
	if err != nil {
		return nil, err
	}

	res := make([][]string, len(columns))
	for i, _ := range columns {
		res[i] = make([]string, len(columns[i]))
		for j, _ := range columns[i] {
			modIdx := columns[i][j]
			modStr := m.moduleGraph.ModuleNameFromIndex(modIdx)
			if !m.Seen[modStr] {
				res[i][j] = Styles.UnavailableModule.Render(modStr)
				continue
			}
			if modIdx == m.Highlighted {
				res[i][j] = Styles.HighlightedModule.Render(modStr)
				continue
			}
			if modIdx == m.Selected {
				res[i][j] = Styles.SelectedModule.Render(modStr)
				continue
			}
			res[i][j] = Styles.UnselectedModule.Render(modStr)
		}
	}

	return res, nil
}
