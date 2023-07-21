package explorer

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/styles"
	"github.com/stretchr/testify/assert"
)

func getTestNavigator() *Navigator {
	modules := newTestModules()
	graph := newTestGraph(modules)

	c := common.Common{
		Styles: styles.DefaultStyles(),
	}

	nav, err := New("X", c, WithModuleGraph(graph))
	if err != nil {
		panic(err)
	}

	return nav
}

func TestNavigator_Init(t *testing.T) {
	nav := getTestNavigator()

	cmd := nav.Init()
	assert.Nil(t, cmd)
}

func TestNavigator_Update_ModuleSelectedMsg(t *testing.T) {
	nav := getTestNavigator()

	msg := common.ModuleSelectedMsg("A")
	_, cmd := nav.Update(msg)

	assert.Equal(t, "A", nav.SelectedModule)
	assert.Equal(t, "A", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Nil(t, cmd)
}

func TestNavigator_Update_Up_Middle(t *testing.T) {
	up := tea.KeyMsg{Type: tea.KeyUp}
	nav := getTestNavigator()

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	_, _ = nav.Update(up)

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Up_InParentColumn(t *testing.T) {
	up := tea.KeyMsg{Type: tea.KeyUp}
	nav := getTestNavigator()

	nav.InParentColumn = true
	nav.HighlightedModule = "B"
	nav.HighlightedIndex = 1
	nav.SelectedModule = "X"

	_, _ = nav.Update(up)

	assert.True(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "A", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	_, _ = nav.Update(up)

	assert.True(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "C", nav.HighlightedModule)
	assert.Equal(t, 2, nav.HighlightedIndex)
}

func TestNavigator_Update_Up_InChildColumn(t *testing.T) {
	up := tea.KeyMsg{Type: tea.KeyUp}
	nav := getTestNavigator()

	nav.InChildColumn = true
	nav.HighlightedModule = "R"
	nav.HighlightedIndex = 1
	nav.SelectedModule = "X"

	_, _ = nav.Update(up)

	assert.False(t, nav.InParentColumn)
	assert.True(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "Q", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	_, _ = nav.Update(up)

	assert.False(t, nav.InParentColumn)
	assert.True(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "S", nav.HighlightedModule)
	assert.Equal(t, 2, nav.HighlightedIndex)
}

func TestNavigator_Update_Down_Middle(t *testing.T) {
	down := tea.KeyMsg{Type: tea.KeyDown}
	nav := getTestNavigator()

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	_, _ = nav.Update(down)

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Down_InParentColumn(t *testing.T) {
	down := tea.KeyMsg{Type: tea.KeyDown}
	nav := getTestNavigator()

	nav.InParentColumn = true
	nav.HighlightedModule = "B"
	nav.HighlightedIndex = 1
	nav.SelectedModule = "X"

	_, _ = nav.Update(down)

	assert.True(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "C", nav.HighlightedModule)
	assert.Equal(t, 2, nav.HighlightedIndex)

	_, _ = nav.Update(down)

	assert.True(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "A", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Down_InChildColumn(t *testing.T) {
	down := tea.KeyMsg{Type: tea.KeyDown}
	nav := getTestNavigator()

	nav.InChildColumn = true
	nav.HighlightedModule = "R"
	nav.HighlightedIndex = 1
	nav.SelectedModule = "X"

	_, _ = nav.Update(down)

	assert.False(t, nav.InParentColumn)
	assert.True(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "S", nav.HighlightedModule)
	assert.Equal(t, 2, nav.HighlightedIndex)

	_, _ = nav.Update(down)

	assert.False(t, nav.InParentColumn)
	assert.True(t, nav.InChildColumn)
	assert.Equal(t, "X", nav.SelectedModule)
	assert.Equal(t, "Q", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Left_InParentColumn_NoEffect(t *testing.T) {
	left := tea.KeyMsg{Type: tea.KeyLeft}
	nav := getTestNavigator()

	nav.InChildColumn = false
	nav.InParentColumn = true
	nav.HighlightedModule = "B"
	nav.HighlightedIndex = 1

	_, _ = nav.Update(left)

	assert.False(t, nav.InChildColumn)
	assert.True(t, nav.InParentColumn)
	assert.Equal(t, "B", nav.HighlightedModule)
	assert.Equal(t, 1, nav.HighlightedIndex)
}

func TestNavigator_Update_Left_InChildColumn(t *testing.T) {
	left := tea.KeyMsg{Type: tea.KeyLeft}
	nav := getTestNavigator()

	nav.InChildColumn = true
	nav.HighlightedModule = "R"
	nav.HighlightedIndex = 1

	_, _ = nav.Update(left)

	assert.False(t, nav.InChildColumn)
	assert.False(t, nav.InParentColumn)
	assert.Equal(t, "X", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Left_InCurrentColumn_WithMemory(t *testing.T) {
	left := tea.KeyMsg{Type: tea.KeyLeft}
	nav := getTestNavigator()

	nav.InChildColumn = false
	nav.InParentColumn = false
	nav.HighlightedModule = "X"
	nav.HighlightedIndex = 0

	nav.memory = NewNavigatorMemory()
	nav.memory.SetLastSelectedParentOf("X", "C")

	_, _ = nav.Update(left)

	assert.False(t, nav.InChildColumn)
	assert.True(t, nav.InParentColumn)
	assert.Equal(t, "C", nav.HighlightedModule)
	assert.Equal(t, 2, nav.HighlightedIndex)
}

func TestNavigator_Update_Left_InCurrentColumn_NoMemory(t *testing.T) {
	left := tea.KeyMsg{Type: tea.KeyLeft}
	nav := getTestNavigator()

	nav.InChildColumn = false
	nav.InParentColumn = false
	nav.HighlightedModule = "X"
	nav.HighlightedIndex = 0

	nav.memory = NewNavigatorMemory()

	_, _ = nav.Update(left)

	assert.False(t, nav.InChildColumn)
	assert.True(t, nav.InParentColumn)
	assert.Equal(t, "A", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Right_InParentColumn(t *testing.T) {
	right := tea.KeyMsg{Type: tea.KeyRight}
	nav := getTestNavigator()

	nav.InChildColumn = false
	nav.InParentColumn = true
	nav.HighlightedModule = "B"
	nav.HighlightedIndex = 1

	_, _ = nav.Update(right)

	assert.False(t, nav.InChildColumn)
	assert.False(t, nav.InParentColumn)
	assert.Equal(t, "X", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Right_InChildColumn_NoEffect(t *testing.T) {
	right := tea.KeyMsg{Type: tea.KeyRight}
	nav := getTestNavigator()

	nav.InParentColumn = false
	nav.InChildColumn = true
	nav.HighlightedModule = "R"
	nav.HighlightedIndex = 1

	_, _ = nav.Update(right)

	assert.True(t, nav.InChildColumn)
	assert.False(t, nav.InParentColumn)
	assert.Equal(t, "R", nav.HighlightedModule)
	assert.Equal(t, 1, nav.HighlightedIndex)
}

func TestNavigator_Update_Right_InCurrentColumn_WithMemory(t *testing.T) {
	right := tea.KeyMsg{Type: tea.KeyRight}
	nav := getTestNavigator()

	nav.InParentColumn = false
	nav.InChildColumn = false
	nav.HighlightedModule = "X"
	nav.HighlightedIndex = 0

	nav.memory = NewNavigatorMemory()
	nav.memory.SetLastSelectedChildOf("X", "R")

	_, _ = nav.Update(right)

	assert.True(t, nav.InChildColumn)
	assert.False(t, nav.InParentColumn)
	assert.Equal(t, "R", nav.HighlightedModule)
	assert.Equal(t, 1, nav.HighlightedIndex)
}

func TestNavigator_Update_Right_InCurrentColumn_NoMemory(t *testing.T) {
	right := tea.KeyMsg{Type: tea.KeyRight}
	nav := getTestNavigator()

	nav.InParentColumn = false
	nav.InChildColumn = false
	nav.HighlightedModule = "X"
	nav.HighlightedIndex = 0

	nav.memory = NewNavigatorMemory()

	_, _ = nav.Update(right)

	assert.True(t, nav.InChildColumn)
	assert.False(t, nav.InParentColumn)
	assert.Equal(t, "Q", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)
}

func TestNavigator_Update_Select_Child(t *testing.T) {
	selectKey := tea.KeyMsg{Type: tea.KeyEnter}
	nav := getTestNavigator()

	nav.InParentColumn = false
	nav.InChildColumn = true
	nav.HighlightedModule = "Q"
	nav.HighlightedIndex = 0

	_, _ = nav.Update(selectKey)

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "Q", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	//check memory
	lsc, ok := nav.memory.GetLastSelectedChildOf("X")
	assert.True(t, ok)
	assert.Equal(t, lsc, "Q")

	lsp, ok := nav.memory.GetLastSelectedParentOf("Q")
	assert.True(t, ok)
	assert.Equal(t, lsp, "X")
}

func TestNavigator_Update_Select_Parent_KnownModule(t *testing.T) {
	selectKey := tea.KeyMsg{Type: tea.KeyEnter}
	nav := getTestNavigator()

	nav.AddModule("A")

	nav.InParentColumn = true
	nav.InChildColumn = false
	nav.HighlightedModule = "A"
	nav.HighlightedIndex = 0

	_, cmd := nav.Update(selectKey)

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "A", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	//check memory
	lsc, ok := nav.memory.GetLastSelectedChildOf("A")
	assert.True(t, ok)
	assert.Equal(t, lsc, "X")

	lsp, ok := nav.memory.GetLastSelectedParentOf("X")
	assert.True(t, ok)
	assert.Equal(t, lsp, "A")

	//check commands
	assert.NotNil(t, cmd)
}

func TestNavigator_Update_Select_Parent_UnknownModule(t *testing.T) {
	selectKey := tea.KeyMsg{Type: tea.KeyEnter}
	nav := getTestNavigator()

	nav.InParentColumn = true
	nav.InChildColumn = false
	nav.HighlightedModule = "A"
	nav.HighlightedIndex = 0

	_, _ = nav.Update(selectKey)

	assert.False(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "A", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	//check memory
	lsc, ok := nav.memory.GetLastSelectedChildOf("A")
	assert.True(t, ok)
	assert.Equal(t, lsc, "X")

	lsp, ok := nav.memory.GetLastSelectedParentOf("X")
	assert.True(t, ok)
	assert.Equal(t, lsp, "A")
}

func TestNavigator_Update_Select_Parent_UnselectableModule(t *testing.T) {
	selectKey := tea.KeyMsg{Type: tea.KeyEnter}
	nav := getTestNavigator()

	nav.InParentColumn = true
	nav.InChildColumn = false
	nav.HighlightedModule = "unselectable"
	nav.HighlightedIndex = 0

	_, cmd := nav.Update(selectKey)

	assert.True(t, nav.InParentColumn)
	assert.False(t, nav.InChildColumn)
	assert.Equal(t, "unselectable", nav.HighlightedModule)
	assert.Equal(t, 0, nav.HighlightedIndex)

	//check commands
	assert.Nil(t, cmd)
}

func TestNavigator_View(t *testing.T) {
	nav := getTestNavigator()
	nav.memory.SetLastSelectedChildOf("X", "R")
	nav.memory.SetLastSelectedParentOf("R", "X")

	nav.View()
}

func TestNavigator_AddModule(t *testing.T) {

}

func newTestModules() []*pbsubstreams.Module {
	return []*pbsubstreams.Module{
		{
			Name: "A",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{
						Type: "blocks.source.v1",
					}},
				},
				{
					Input: &pbsubstreams.Module_Input_Params_{Params: &pbsubstreams.Module_Input_Params{
						Value: "params.value.xyz",
					}},
				},
			},
		},
		{
			Name: "B",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{
						Type: "blocks.source.v1",
					}},
				},
			},
		},
		{
			Name: "C",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{
						Type: "blocks.source.v1",
					}},
				},
			},
		},
		{
			Name: "X",
			Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "A",
					}},
				},
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "B",
					}},
				},
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "C",
					}},
				},
			},
		},
		{
			Name: "Q",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{
						Type: "blocks.source.v1",
					}},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "X",
					}},
				},
			},
		},
		{
			Name: "R",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "X",
					}},
				},
			},
		},
		{
			Name: "S",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "X",
					}},
				},
			},
		},
		{
			Name: "Z",
			Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "Q",
					}},
				},
			},
		},
	}
}

func newTestGraph(modules []*pbsubstreams.Module) *manifest.ModuleGraph {
	graph, err := manifest.NewModuleGraph(modules)
	if err != nil {
		panic(err)
	}
	return graph
}
