package explorer

import (
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
)

type NavigatorMemory struct {
	lastSelectedParentOf map[string]string
	lastSelectedChildOf  map[string]string

	lock sync.RWMutex
}

func NewNavigatorMemory() *NavigatorMemory {
	return &NavigatorMemory{
		lastSelectedParentOf: map[string]string{},
		lastSelectedChildOf:  map[string]string{},
	}
}

func (n *NavigatorMemory) GetLastSelectedParentOf(modName string) (string, bool) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	v, ok := n.lastSelectedParentOf[modName]
	return v, ok
}

func (n *NavigatorMemory) SetLastSelectedParentOf(modName, parentName string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.lastSelectedParentOf[modName] = parentName
}

func (n *NavigatorMemory) GetLastSelectedChildOf(modName string) (string, bool) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	v, ok := n.lastSelectedChildOf[modName]
	return v, ok
}

func (n *NavigatorMemory) SetLastSelectedChildOf(modName, childName string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.lastSelectedChildOf[modName] = childName
}

type Navigator struct {
	common.Common
	graph  *manifest.ModuleGraph
	memory *NavigatorMemory

	SelectedModule    string
	HighlightedModule string
	HighlightedIndex  int

	InParentColumn bool
	InChildColumn  bool

	CurrentGrandParentPreviewColumn []string
	CurrentParentColumn             []string
	CurrentChildColumn              []string
	CurrentGrandChildPreviewColumn  []string

	CurrentPreviewColumn []string

	selectableModules map[string]bool
	knownModules      map[string]bool
	mutex             sync.RWMutex

	longestModuleName int
	FrameHeight       int
}

type Option func(*Navigator)

func WithModuleGraph(graph *manifest.ModuleGraph) Option {
	return func(n *Navigator) {
		n.graph = graph
	}
}

func New(requestOutputModule string, c common.Common, opts ...Option) (*Navigator, error) {
	n := &Navigator{
		Common:            c,
		knownModules:      map[string]bool{},
		selectableModules: map[string]bool{},
		memory:            NewNavigatorMemory(),
	}

	for _, opt := range opts {
		opt(n)
	}

	if n.graph == nil {
		return nil, fmt.Errorf("no module graph provided")
	}

	n.mutex.Lock()
	for _, mod := range n.graph.Modules() {
		n.selectableModules[mod] = true
		if len(mod) > n.longestModuleName {
			n.longestModuleName = len(mod)
		}
	}
	n.mutex.Unlock()

	//populate initial state
	parents, children, err := n.graph.Context(requestOutputModule)
	if err != nil {
		return nil, err
	}

	n.CurrentParentColumn = parents
	n.CurrentChildColumn = children
	n.SelectedModule = requestOutputModule
	n.HighlightedModule = requestOutputModule

	return n, nil
}

// TODO(colin): call this from somewhere. same place as where the modselect.AddModule is called
func (n *Navigator) AddModule(modName string) {
	n.mutex.RLock()
	if n.knownModules[modName] {
		n.mutex.RUnlock()
		return
	}
	n.mutex.RUnlock()

	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.knownModules[modName] = true
}

func (n *Navigator) Init() tea.Cmd { return nil }

func (n *Navigator) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case common.ModuleSelectedMsg:
		newModule := string(msg)
		parents, children, err := n.graph.Context(newModule)
		if err != nil {
			break
		}

		n.SelectedModule = newModule
		n.HighlightedModule = newModule
		n.InParentColumn = false
		n.InChildColumn = false
		n.HighlightedIndex = 0

		n.CurrentParentColumn = parents
		n.CurrentChildColumn = children

		n.CurrentGrandChildPreviewColumn = []string{}
		n.CurrentGrandParentPreviewColumn = []string{}
		n.CurrentPreviewColumn = []string{}
	case tea.KeyMsg:
		switch msg.String() {
		case "left":
			if n.InParentColumn {
				break // already in the left-most column
			} else if n.InChildColumn {
				// move to the "current" column
				n.InChildColumn = false
				n.InParentColumn = false
				n.HighlightedIndex = 0
				n.HighlightedModule = n.SelectedModule

				n.CurrentGrandChildPreviewColumn = []string{}
				n.CurrentGrandParentPreviewColumn = []string{}
				n.CurrentPreviewColumn = []string{}
			} else {
				// if parent column is nil, do nothing
				if n.CurrentParentColumn == nil || len(n.CurrentParentColumn) == 0 {
					break
				}

				// move to the parent column
				n.InChildColumn = false
				n.InParentColumn = true

				//highlight last selected parent of current highlighted module
				lsp, ok := n.memory.GetLastSelectedParentOf(n.HighlightedModule)
				if !ok {
					n.HighlightedModule = n.CurrentParentColumn[0]
					n.HighlightedIndex = 0
				} else {
					n.HighlightedModule = lsp
					for i, p := range n.CurrentParentColumn {
						if p == lsp {
							n.HighlightedIndex = i
							break
						}
					}
				}

				grandparents, currentPreview, _ := n.graph.Context(n.HighlightedModule)
				n.CurrentGrandParentPreviewColumn, n.CurrentPreviewColumn = grandparents, currentPreview
			}
		case "right":
			if n.InChildColumn {
				break // already in the right-most column
			} else if n.InParentColumn {
				// move to the current column
				n.InChildColumn = false
				n.InParentColumn = false
				n.HighlightedIndex = 0
				n.HighlightedModule = n.SelectedModule

				n.CurrentGrandChildPreviewColumn = []string{}
				n.CurrentGrandParentPreviewColumn = []string{}
				n.CurrentPreviewColumn = []string{}
			} else {
				if n.CurrentChildColumn == nil || len(n.CurrentChildColumn) == 0 {
					break
				}

				// move to the child column
				n.InChildColumn = true
				n.InParentColumn = false

				//highlight last selected child
				lsc, ok := n.memory.GetLastSelectedChildOf(n.HighlightedModule)
				if !ok {
					n.HighlightedModule = n.CurrentChildColumn[0]
					n.HighlightedIndex = 0
				} else {
					n.HighlightedModule = lsc
					for i, c := range n.CurrentChildColumn {
						if c == lsc {
							n.HighlightedIndex = i
							break
						}
					}
				}

				currentPreview, grandchildren, _ := n.graph.Context(n.HighlightedModule)
				n.CurrentGrandChildPreviewColumn, n.CurrentPreviewColumn = grandchildren, currentPreview
			}
		case "up":
			if n.InParentColumn {
				// move up in the parent column
				// if we're at the top, move to the bottom
				newIndex := n.HighlightedIndex - 1
				if newIndex < 0 {
					newIndex = len(n.CurrentParentColumn) - 1
				}
				n.HighlightedModule = n.CurrentParentColumn[newIndex]
				n.HighlightedIndex = newIndex

				grandparents, currentPreview, _ := n.graph.Context(n.HighlightedModule)
				n.CurrentGrandParentPreviewColumn, n.CurrentPreviewColumn = grandparents, currentPreview
			} else if n.InChildColumn {
				// move up in the parent column
				// if we're at the top, move to the bottom
				newIndex := n.HighlightedIndex - 1
				if newIndex < 0 {
					newIndex = len(n.CurrentChildColumn) - 1
				}

				n.HighlightedModule = n.CurrentChildColumn[newIndex]
				n.HighlightedIndex = newIndex

				currentPreview, grandchildren, _ := n.graph.Context(n.HighlightedModule)
				n.CurrentGrandChildPreviewColumn, n.CurrentPreviewColumn = grandchildren, currentPreview
			} else {
				break // nothing to do
			}
		case "down":
			if n.InParentColumn {
				// move down in the parent column
				// if we're at the bottom, move to the top
				newIndex := n.HighlightedIndex + 1
				if newIndex >= len(n.CurrentParentColumn) {
					newIndex = 0
				}
				n.HighlightedModule = n.CurrentParentColumn[newIndex]
				n.HighlightedIndex = newIndex

				grandparents, currentPreview, _ := n.graph.Context(n.HighlightedModule)
				n.CurrentGrandParentPreviewColumn, n.CurrentPreviewColumn = grandparents, currentPreview
			} else if n.InChildColumn {
				// move down in the child column
				// if we're at the bottom, move to the top
				newIndex := n.HighlightedIndex + 1
				if newIndex >= len(n.CurrentChildColumn) {
					newIndex = 0
				}
				n.HighlightedModule = n.CurrentChildColumn[newIndex]
				n.HighlightedIndex = newIndex

				currentPreview, grandchildren, _ := n.graph.Context(n.HighlightedModule)
				n.CurrentGrandChildPreviewColumn, n.CurrentPreviewColumn = grandchildren, currentPreview
			} else {
				break // nothing to do
			}
		case "enter":
			// select the highlighted module, dispatch if it is a known module
			if n.knownModules[n.HighlightedModule] {
				cmds = append(cmds, n.dispatchModuleSelected)
			}

			// check if this module is selectable?
			if _, ok := n.selectableModules[n.HighlightedModule]; !ok {
				break
			}

			parents, children, err := n.graph.Context(n.HighlightedModule)
			if err != nil {
				break
			}

			// update the memory links
			if n.InParentColumn {
				n.memory.SetLastSelectedChildOf(n.HighlightedModule, n.SelectedModule)
				n.memory.SetLastSelectedParentOf(n.SelectedModule, n.HighlightedModule)
			} else if n.InChildColumn {
				n.memory.SetLastSelectedParentOf(n.HighlightedModule, n.SelectedModule)
				n.memory.SetLastSelectedChildOf(n.SelectedModule, n.HighlightedModule)
			}

			n.SelectedModule = n.HighlightedModule
			n.InParentColumn = false
			n.InChildColumn = false
			n.HighlightedIndex = 0

			n.CurrentParentColumn = parents
			n.CurrentChildColumn = children

			n.CurrentGrandChildPreviewColumn = []string{}
			n.CurrentGrandParentPreviewColumn = []string{}
			n.CurrentPreviewColumn = []string{}

			cmds = append(cmds, common.EmitModuleSelectedMsg(n.SelectedModule))
		}
	}

	return n, tea.Batch(cmds...)
}

func (n *Navigator) dispatchModuleSelected() tea.Msg {
	return common.EmitModuleSelectedMsg(n.HighlightedModule)
}

func tallestCol(arrs ...[]string) int {
	max := 0

	for _, arr := range arrs {
		if len(arr) > max {
			max = len(arr)
		}
	}

	return max
}

func (n *Navigator) getRemainingLines(occupiedHeight int) string {
	str := ""
	for i := 0; i < n.FrameHeight-occupiedHeight-1; i++ {
		str += "\n"
	}
	return str
}

func (n *Navigator) View() string {
	//generate a string of spaces of length longestModuleName
	spaces := strings.Repeat(" ", n.longestModuleName+10)

	// get copies of the current columns
	parents := make([]string, len(n.CurrentParentColumn))
	copy(parents, n.CurrentParentColumn)

	children := make([]string, len(n.CurrentChildColumn))
	copy(children, n.CurrentChildColumn)

	var current string
	current = strings.Clone(n.SelectedModule)

	grandparents := make([]string, len(n.CurrentGrandParentPreviewColumn))
	copy(grandparents, n.CurrentGrandParentPreviewColumn)

	grandchildren := make([]string, len(n.CurrentGrandChildPreviewColumn))
	copy(grandchildren, n.CurrentGrandChildPreviewColumn)

	preview := make([]string, len(n.CurrentPreviewColumn))
	copy(preview, n.CurrentPreviewColumn)

	var parent_arrow_idx int
	lcp, _ := n.memory.GetLastSelectedParentOf(n.SelectedModule)
	for i, p := range parents {
		op := p

		// figure out where the arrow will go
		if lcp != "" && op == lcp {
			parent_arrow_idx = i
		}

		// add brackets around the current highlighted module
		if op == n.HighlightedModule {
			p = " [" + p + "] "
		} else {
			p = "  " + p + "  "
		}

		// colorize the module
		if op == n.SelectedModule {
			parents[i] = n.Styles.Navigator.SelectedModule.Render(p)
		} else if op == n.HighlightedModule {
			parents[i] = n.Styles.Navigator.HighlightedModule.Render(p)
		} else {
			selectable := n.selectableModules[op]
			if selectable {
				parents[i] = n.Styles.Navigator.SelectableModule.Render(p)
			} else {
				parents[i] = n.Styles.Navigator.UnselectableModule.Render(p)
			}
		}
	}

	// add the arrow to the parent column
	for i, p := range parents {
		if i == parent_arrow_idx {
			parents[i] = p + " -> "
		} else {
			parents[i] = p + "   "
		}
	}

	var child_arrow_idx int
	lcc, _ := n.memory.GetLastSelectedChildOf(n.SelectedModule)
	for i, c := range children {
		oc := c

		// figure out where the arrow will go
		if lcc != "" && oc == lcc {
			child_arrow_idx = i
		}

		// add brackets around the current highlighted module
		if oc == n.HighlightedModule {
			c = " [" + c + "] "
		} else {
			c = " " + c + " "
		}

		// colorize the module
		if oc == n.SelectedModule {
			children[i] = n.Styles.Navigator.SelectedModule.Render(c)
		} else if oc == n.HighlightedModule {
			children[i] = n.Styles.Navigator.HighlightedModule.Render(c)
		} else {
			selectable := n.selectableModules[oc]
			if selectable {
				children[i] = n.Styles.Navigator.SelectableModule.Render(c)
			} else {
				children[i] = n.Styles.Navigator.UnselectableModule.Render(c)
			}
		}
	}

	// add the arrow to the child column
	for i, c := range children {
		if i == child_arrow_idx {
			children[i] = " -> " + c
		} else {
			children[i] = "    " + c
		}
	}

	if len(preview) > 0 {
		// add brackets around the current highlighted module
		// render the rest with the preview style
		for i, p := range preview {
			if p == n.HighlightedModule {
				if p == n.SelectedModule {
					preview[i] = " [" + n.Styles.Navigator.SelectedModule.Render(p) + "] "
				} else {
					preview[i] = " [" + n.Styles.Navigator.HighlightedModule.Render(p) + "] "
				}
			} else if p == n.SelectedModule {
				preview[i] = " " + n.Styles.Navigator.SelectedModule.Render(p) + " "
			} else {
				preview[i] = n.Styles.Navigator.Preview.Render(p)
			}
		}
		current = lipgloss.JoinVertical(lipgloss.Center, preview...)
	} else {
		if current == n.HighlightedModule {
			current = n.Styles.Navigator.HighlightedModule.Render(" [") + n.Styles.Navigator.SelectedModule.Render(current) + n.Styles.Navigator.HighlightedModule.Render("] ")
		} else {
			current = n.Styles.Navigator.SelectedModule.Render(" " + current + " ")
		}
	}

	for i, g := range grandparents {
		grandparents[i] = n.Styles.Navigator.Preview.Render(g)
	}

	for i, g := range grandchildren {
		grandchildren[i] = n.Styles.Navigator.Preview.Render(g)
	}

	var leftPreviewSide string
	var leftSide string
	var middle string
	var rightSide string
	var rightPreviewSide string

	if n.InChildColumn {
		leftPreviewSide = lipgloss.JoinVertical(lipgloss.Right, append([]string{spaces}, parents...)...)

		var inputsWithLabel []string
		if len(parents) > 0 {
			inputsWithLabel = []string{"INPUTS", spaces, current}
		} else {
			inputsWithLabel = []string{"\n", spaces}
		}
		leftSide = lipgloss.JoinVertical(lipgloss.Center, append([]string{spaces}, inputsWithLabel...)...)

		currentWithLabel := append([]string{"CURRENT", spaces}, children...)
		middle = lipgloss.JoinVertical(lipgloss.Center, currentWithLabel...)

		var consumersWithLabel []string
		if len(grandchildren) > 0 {
			consumersWithLabel = append([]string{"CONSUMERS", spaces}, grandchildren...)
		} else {
			consumersWithLabel = []string{"\n", spaces}
		}
		rightSide = lipgloss.JoinVertical(lipgloss.Center, consumersWithLabel...)
		rightPreviewSide = lipgloss.JoinVertical(lipgloss.Left, spaces)
	} else if n.InParentColumn {
		leftPreviewSide = lipgloss.JoinVertical(lipgloss.Right, spaces)

		var inputsWithLabel []string
		if len(grandparents) > 0 {
			inputsWithLabel = append([]string{"INPUTS", spaces}, grandparents...)
		} else {
			inputsWithLabel = []string{"\n", spaces}
		}
		leftSide = lipgloss.JoinVertical(lipgloss.Center, inputsWithLabel...)

		currentWithLabel := append([]string{"CURRENT", spaces}, parents...)
		middle = lipgloss.JoinVertical(lipgloss.Center, currentWithLabel...)

		var consumersWithLabel []string
		if len(current) > 0 {
			consumersWithLabel = []string{"CONSUMERS", spaces, current}
		} else {
			consumersWithLabel = []string{"\n", spaces}
		}
		rightSide = lipgloss.JoinVertical(lipgloss.Center, consumersWithLabel...)

		rightPreviewSide = lipgloss.JoinVertical(lipgloss.Left, append([]string{spaces}, children...)...)
	} else {
		//draw the grandparent column
		leftPreviewSide = lipgloss.JoinVertical(lipgloss.Right, append([]string{spaces}, grandparents...)...)

		//draw the parent column
		var inputsWithLabel []string
		if len(parents) > 0 {
			inputsWithLabel = append([]string{"INPUTS", spaces}, parents...)
		} else {
			inputsWithLabel = []string{"\n", spaces}
		}
		leftSide = lipgloss.JoinVertical(lipgloss.Center, inputsWithLabel...)

		//draw the current module
		currentWithLabel := append([]string{"CURRENT", spaces}, current)
		middle = lipgloss.JoinVertical(lipgloss.Center, currentWithLabel...)

		//draw the child column
		var consumersWithLabel []string
		if len(children) > 0 {
			consumersWithLabel = append([]string{"CONSUMERS", spaces}, children...)
		}
		rightSide = lipgloss.JoinVertical(lipgloss.Center, consumersWithLabel...)

		//draw the grandchild column
		rightPreviewSide = lipgloss.JoinVertical(lipgloss.Left, append([]string{spaces}, grandchildren...)...)
	}

	verticalSpace := tallestCol(parents, children, grandparents, grandchildren, preview)
	content := lipgloss.JoinHorizontal(lipgloss.Center,
		leftPreviewSide,
		leftSide,
		middle,
		rightSide,
		rightPreviewSide,
	)
	res := lipgloss.JoinVertical(0,
		content,
		n.getRemainingLines(verticalSpace),
	)

	return res
}
