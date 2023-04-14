package search

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/streamingfast/substreams/tui2/common"
)

type SearchClearedMsg bool
type ApplySearchQueryMsg string
type JumpToNextMatchMsg jumpToMatch
type JumpToPreviousMatchMsg jumpToMatch

type jumpToMatch []int

type Search struct {
	input textinput.Model

	enabled        bool
	Query          string
	timesFound     int
	matchPositions []int
}

func New() *Search {
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
			case "N":
				cmds = append(cmds, s.jumpToPreviousMatch())
			case "n":
				cmds = append(cmds, s.jumpToNextMatch())
				//case "O":
				//	cmds = append(cmds, s.jumpToPreviousMatchingBlock())
				//case "P":
				//	cmds = append(cmds, s.jumpToNextMatchingBlock())
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

func (s *Search) SetPositions(positions []int) {
	s.matchPositions = positions
}

func (s *Search) jumpToNextMatch() tea.Cmd {
	return func() tea.Msg {
		return JumpToNextMatchMsg(s.matchPositions)
	}
}

func (s *Search) jumpToPreviousMatch() tea.Cmd {
	return func() tea.Msg {
		return JumpToPreviousMatchMsg(s.matchPositions)
	}
}

func (s *Search) applySearchQuery(query string) tea.Cmd {
	return func() tea.Msg {
		return ApplySearchQueryMsg(query)
	}
}
