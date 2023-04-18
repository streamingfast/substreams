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

	outputModule  int
	columns       [][]int
	locationIndex map[int][2]int

	Selected             int
	SelectedColumn       int
	SelectedColumnIndex  int
	SelectedColumnLength int

	Highlighted             int
	HighlightedColumn       int
	HighlightedColumnIndex  int
	HighlightedColumnLength int

	moduleGraph *manifest.ModuleGraph
}

func New(c common.Common, manifestPath string, outputModule string) *ModSelect {
	g := manifest.MustNewModuleGraph(manifest.NewReader(manifestPath).MustRead().Modules.Modules)
	modules := g.Modules()

	if outputModule == "" {
		panic("output module is empty")
	}

	return &ModSelect{
		Common:       c,
		Seen:         map[string]bool{},
		Modules:      modules,
		outputModule: g.ModuleIndexFromName(outputModule),

		locationIndex: make(map[int][2]int),

		moduleGraph: g,
	}
}

func newTestModSelect(modules []*pbsubstreams.Module) *ModSelect {
	g := manifest.MustNewModuleGraph(modules)

	return &ModSelect{
		Common:       common.Common{},
		Seen:         map[string]bool{},
		Modules:      g.Modules(),
		outputModule: 4,

		locationIndex: make(map[int][2]int),

		moduleGraph: g,
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
		case "a":
			// go up in the current column
			m.HighlightedColumnIndex--
			if m.HighlightedColumnIndex < 0 {
				m.HighlightedColumnIndex = m.HighlightedColumnLength - 1 // wrap around
			}
			m.Highlighted = m.columns[m.HighlightedColumn][m.HighlightedColumnIndex]
		case "z":
			// go down in the current column
			m.HighlightedColumnIndex++
			if m.HighlightedColumnIndex >= m.HighlightedColumnLength {
				m.HighlightedColumnIndex = 0 // wrap around
			}
			m.Highlighted = m.columns[m.HighlightedColumn][m.HighlightedColumnIndex]
		case "i":
			// go right to the next column
			if m.HighlightedColumn+1 >= len(m.columns) {
				break //cannot go right. already at the end
			}

			m.HighlightedColumn++
			m.HighlightedColumnIndex = 0
			m.HighlightedColumnLength = len(m.columns[m.HighlightedColumn])
			m.Highlighted = m.columns[m.HighlightedColumn][m.HighlightedColumnIndex]
		case "u":
			// go left to the previous column
			if m.HighlightedColumn-1 < 0 {
				break //cannot go left. already at the beginning
			}

			m.HighlightedColumn--
			m.HighlightedColumnIndex = 0
			m.HighlightedColumnLength = len(m.columns[m.HighlightedColumn])
			m.Highlighted = m.columns[m.HighlightedColumn][m.HighlightedColumnIndex]
		case "b":
			if m.Highlighted == m.Selected {
				break // nothing to do
			}

			//redraw all the things!
			// reset the columns and location index
			_, err := m.SetColumns()
			if err != nil {
				panic(err)
			}

			loc := m.locationIndex[m.Selected]
			m.SelectedColumn = loc[0]
			m.SelectedColumnIndex = loc[1]
			m.HighlightedColumn = m.SelectedColumn
			m.HighlightedColumnIndex = m.SelectedColumnIndex

			cmds = append(cmds, m.dispatchModuleSelected)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m *ModSelect) AddModule(modName string) {
	if !m.Seen[modName] {
		isFirstDataModule := len(m.Seen) == 0
		m.Seen[modName] = true

		if isFirstDataModule {
			ix := m.moduleGraph.ModuleIndexFromName(modName)
			m.Selected = ix
			m.Highlighted = m.Selected

			_, err := m.SetColumns()
			if err != nil {
				panic(err)
			}

			loc := m.locationIndex[m.Selected]
			m.SelectedColumn = loc[0]
			m.SelectedColumnIndex = loc[1]
			m.HighlightedColumn = m.SelectedColumn
			m.HighlightedColumnIndex = m.SelectedColumnIndex
		}
	}
}

func (m *ModSelect) dispatchModuleSelected() tea.Msg {
	return ModuleSelectedMsg(m.Modules[m.Selected])
}

func (m *ModSelect) View() string {
	if len(m.Seen) == 0 {
		return ""
	}

	cs, _ := m.GetRenderedColumns()

	var leftSide, rightSide []Column
	var center Column
	leftSide, center, rightSide = cs[:m.SelectedColumn], cs[m.SelectedColumn], cs[m.SelectedColumn+1:]

	_, _, _ = leftSide, center, rightSide

	return ""

	//var firstPart, lastPart, tmp []string
	//var activeModule string
	//for idx, mod := range m.Modules {
	//	if idx == m.Selected {
	//		activeModule = mod
	//		firstPart = tmp[:]
	//		tmp = nil
	//	} else {
	//		tmp = append(tmp, mod)
	//	}
	//}
	//lastPart = tmp
	//
	//sidePartsWidth := (m.Width-len(activeModule)-2)/2 - 3
	//
	//leftModules := strings.Join(firstPart, "  ")
	//leftWidth := len(leftModules)
	//if leftWidth > sidePartsWidth {
	//	leftModules = "..." + leftModules[leftWidth-sidePartsWidth:]
	//}
	//
	//rightModules := strings.Join(lastPart, "  ")
	//rightWidth := len(rightModules)
	//if rightWidth > sidePartsWidth {
	//	rightModules = rightModules[:sidePartsWidth] + "..."
	//}
	//
	//alignRight := lipgloss.NewStyle().Width(sidePartsWidth + 4).Align(lipgloss.Right)
	//alignLeft := lipgloss.NewStyle().Width(sidePartsWidth + 4).Align(lipgloss.Left)
	//return Styles.Box.MaxWidth(m.Width).Render(
	//	lipgloss.JoinHorizontal(0.5,
	//		alignRight.Render(leftModules),
	//		Styles.SelectedModule.Render(activeModule),
	//		alignLeft.Render(rightModules),
	//	),
	//)
}

func (m *ModSelect) GetColumns() ([][]int, error) {
	if m.columns == nil {
		_, err := m.SetColumns()
		if err != nil {
			return nil, err
		}
	}

	return m.columns, nil
}

func (m *ModSelect) SetColumns() ([][]int, error) {
	g := m.moduleGraph
	_, distances := graph.ShortestPaths(g, m.Selected)

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

	m.columns = res

	m.locationIndex = make(map[int][2]int)
	for i, col := range res {
		for j, modIdx := range col {
			m.locationIndex[modIdx] = [2]int{i, j}
		}
	}

	return res, nil
}

func (m *ModSelect) GetRenderedColumns() ([]Column, error) {
	columns, err := m.GetColumns()
	if err != nil {
		return nil, err
	}

	///trim max height and width

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

	var finalRes []Column
	for _, col := range res {
		finalRes = append(finalRes, col)
	}

	return finalRes, nil
}

type Column []string

func (c Column) Width() int {
	longest := 0
	for _, v := range c {
		if len(v) > longest {
			longest = len(v)
		}
	}
	return longest
}

func (c Column) Height() int {
	return len(c)
}

func (c Column) String(maxHeight, maxWidth int) string {
	return strings.Join(c, "\n")
}

func (c Column) Render(selected, maxHeight, maxWidth int) string {
	//up := "▲"
	//down := "▼"

	return ""
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
