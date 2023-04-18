package search

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/streamingfast/substreams/tui2/common"
)

type UpdateMatchingBlocks map[uint64]bool
type SearchClearedMsg bool
type ApplySearchQueryMsg string

type Search struct {
	input textinput.Model

	enabled    bool
	Query      string
	timesFound int
}

func NewSearch() *Search {
	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "/"
	input.CharLimit = 256
	input.Width = 80
	return &Search{
		input: input,
	}
}

func (s *Search) Init() tea.Cmd {
	return nil
}

func (s *Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.input.Focused() {
			switch msg.String() {
			case "enter":
				s.Query = s.input.Value()
				s.input.Blur()
				cmds = append(cmds, s.cancelModal(), s.applySearchQuery(s.Query))
			case "backspace":
				if s.input.Value() == "" {
					cmds = append(cmds, s.cancelModal(), s.clearSearch)
				}
			}

			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			cmds = append(cmds, cmd)

		} else {
			switch msg.String() {

			}
		}
	}

	return s, tea.Batch(cmds...)
}

func (s *Search) View() string {
	if !s.input.Focused() {
		return fmt.Sprintf("/%s - (%v instances found)", s.Query, s.timesFound)
	} else {
		return s.input.View()
	}
}

func (s *Search) clearSearch() tea.Msg {
	return SearchClearedMsg(true)
}

func (s *Search) InitInput() tea.Cmd {
	s.input.Focus()
	s.input.SetValue("")
	return func() tea.Msg {
		return common.SetModalUpdateFuncMsg(s.Update)
	}
}

func (s *Search) cancelModal() tea.Cmd {
	return func() tea.Msg {
		return common.SetModalUpdateFuncMsg(nil)
	}
}

func (s *Search) SetMatchCount(count int) {
	s.timesFound = count
}

func (s *Search) applySearchQuery(query string) tea.Cmd {
	return func() tea.Msg {
		return ApplySearchQueryMsg(query)
	}
}
