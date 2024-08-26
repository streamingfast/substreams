package modsearch

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/keymap"
)

type ModuleSearch struct {
	list.Model
}

func New(c common.Common, target string) *ModuleSearch {
	delegate := list.NewDefaultDelegate()
	delegate.UpdateFunc = func(msg tea.Msg, m *list.Model) tea.Cmd {
		it, ok := m.SelectedItem().(item)
		if !ok {
			return nil
		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				return tea.Sequence(common.EmitModuleSelectedCmd(it.Title(), target), common.CancelModalCmd())
			}
		}
		return nil
	}
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	lst := list.New(nil, delegate, c.Width, c.Height)
	lst.SetStatusBarItemName("module", "modules")
	lst.Title = "Select module (/ to filter)"
	//lst.SetFilteringEnabled(true)
	lst.DisableQuitKeybindings()
	lst.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keymap.EscapeModal}
	}
	mod := &ModuleSearch{
		Model: lst,
	}
	mod.SetSize(c.Width, c.Height)
	return mod
}

func (m *ModuleSearch) Init() tea.Cmd {
	return nil
}

func (m *ModuleSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.Model.FilterState() == list.Filtering {
				break
			}
			return m, common.CancelModalCmd()
		}
	}

	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}

func (m *ModuleSearch) View() string {
	return m.Model.View()
}

func (m *ModuleSearch) SetSize(w, h int) {
	m.Model.SetSize(
		min(w-12, 60),
		min(h-6, 20)-3,
	)
}
func (m *ModuleSearch) GetWidth() int  { return m.Model.Width() }
func (m *ModuleSearch) GetHeight() int { return m.Model.Height() }

type item struct {
	modName string
}

func (i item) FilterValue() string { return i.modName }
func (i item) Title() string       { return i.modName }
func (i item) Description() string { return "none" }

func (m *ModuleSearch) SetListItems(mods []string) {
	var matchingMods []list.Item
	for _, mod := range mods {
		matchingMods = append(matchingMods, item{modName: mod})
	}
	m.Model.SetItems(matchingMods)
	m.Model.SettingFilter()
}

func (m *ModuleSearch) SetSelected(moduleName string) {
	for idx, it := range m.Model.Items() {
		if it.(item).modName == moduleName {
			m.Model.Select(idx)
			return
		}
	}
}
