package tabs

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/styles"
)

// SelectTabMsg is a message that contains the index of the tab to select.
type SelectTabMsg int

// ActiveTabMsg is a message that contains the index of the current active tab.
type ActiveTabMsg int

// Tabs is bubbletea component that displays a list of tabs.
type Tabs struct {
	common.Common

	tabs         []string
	activeTab    int
	TabSeparator lipgloss.Style
	TabInactive  lipgloss.Style
	TabActive    lipgloss.Style
	TabDot       lipgloss.Style
	UseDot       bool
	LogoStyle    lipgloss.Style
}

// New creates a new Tabs component.
func New(c common.Common, tabs []string) *Tabs {
	r := &Tabs{
		Common:       c,
		tabs:         tabs,
		activeTab:    0,
		TabSeparator: styles.TabSeparator,
		TabInactive:  styles.TabInactive,
		TabActive:    styles.TabActive,
	}
	r.Height = 2
	return r
}

// Init implements tea.Model.
func (t *Tabs) Init() tea.Cmd {
	t.activeTab = 0
	return nil
}

// Update implements tea.Model.
func (t *Tabs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "right":
			t.activeTab = (t.activeTab + 1) % len(t.tabs)
			cmds = append(cmds, t.activeTabCmd)
		case "shift+tab", "left":
			t.activeTab = (t.activeTab - 1 + len(t.tabs)) % len(t.tabs)
			cmds = append(cmds, t.activeTabCmd)
		}
	case SelectTabMsg:
		tab := int(msg)
		if tab >= 0 && tab < len(t.tabs) {
			t.activeTab = int(msg)
		}
	}
	return t, tea.Batch(cmds...)
}

func (t *Tabs) View() string {
	var tabs []string

	for i, tab := range t.tabs {
		label := styles.TabLabel.Render(tab)
		if i == t.activeTab {
			tabs = append(tabs, styles.TabActive.Render(label))
		} else {
			tabs = append(tabs, styles.TabInactive.Render(label))
		}
	}
	tabsView := lipgloss.JoinHorizontal(1.0, tabs...)

	/*
	            ▄▄
	        ▄▄██▀▀
	   ▁▄▄██▀▀ ▁▄▄
	   ▔▀▀█▆▄▄ ▔▀▀██▄▄
	        ▀▀██▄▄▁ ▀▀██▄▄▁
	            ▀▀▔ ▄▄▆█▀▀▔
	            ▄▄██▀▀
	            ▀▀
	*/

	logoTab := t.LogoStyle.Render("Substreams GUI")
	fill := max(t.Width-lipgloss.Width(tabsView)-lipgloss.Width(logoTab)+1, 0)

	return lipgloss.JoinHorizontal(1.0, logoTab, tabsView, strings.Repeat("─", fill))
}

func (t *Tabs) activeTabCmd() tea.Msg {
	return ActiveTabMsg(t.activeTab)
}

// SelectTabCmd is a bubbletea command that selects the tab at the given index.
func SelectTabCmd(tab int) tea.Cmd {
	return func() tea.Msg {
		return SelectTabMsg(tab)
	}
}
