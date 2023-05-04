package search

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/streamingfast/substreams/tui2/common"
)

type UpdateMatchingBlocks map[uint64]bool
type AddMatchingBlock uint64
type SearchClearedMsg bool
type ApplySearchQueryMsg SearchQuery

type SearchQuery struct {
	Query  string
	JQMode bool
}

type Search struct {
	common.Common

	input textinput.Model

	jqMode bool

	History        []SearchQuery
	historyPointer int
	Current        SearchQuery

	timesFound int
}

func New(c common.Common) *Search {
	input := textinput.New()
	input.Placeholder = ""
	input.CharLimit = 1024
	input.Width = c.Width
	return &Search{
		Common: c,
		input:  input,
	}
}

func (s *Search) SetSize(w, h int) {
	s.Common.SetSize(w, h)
	s.input.Width = w
}
func (s *Search) Init() tea.Cmd {
	return nil
}

func (s *Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
msgSwitch:
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.input.Focused() {
			switch msg.String() {
			case "/":
				if s.input.Value() == "" {
					s.jqMode = true
					s.applyPrompt()
					break msgSwitch
				}
			case "up":
				l := len(s.History)
				if s.historyPointer+1 > l {
					break msgSwitch
				}
				s.historyPointer++
				edit := s.History[l-s.historyPointer]
				s.input.SetValue(edit.Query)
				s.jqMode = edit.JQMode
				s.applyPrompt()
			case "down":
				l := len(s.History)
				if s.historyPointer-1 <= 0 {
					break msgSwitch
				}
				s.historyPointer--
				edit := s.History[l-s.historyPointer]
				s.input.SetValue(edit.Query)
				s.jqMode = edit.JQMode
				s.applyPrompt()
			case "enter":
				s.input.Blur()
				newQuery := SearchQuery{
					Query:  s.input.Value(),
					JQMode: s.jqMode,
				}

				cmds = append(cmds, s.CancelModal())

				if newQuery.Query != "" {
					s.Current = newQuery
					s.History = append(s.History, newQuery)
					cmds = append(cmds, s.applySearchQuery(newQuery))
				} else {
					cmds = append(cmds, s.clearSearch)
				}

				break msgSwitch
			case "backspace":
				if s.input.Value() == "" {
					s.input.Blur()
					cmds = append(cmds, s.CancelModal(), s.clearSearch)
				}
			}

			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return s, tea.Batch(cmds...)
}

func (s *Search) applyPrompt() {
	if s.jqMode {
		s.input.Prompt = "jq: "
	} else {
		s.input.Prompt = "/"
	}
}

func (s *Search) View() string {
	if !s.input.Focused() {
		return fmt.Sprintf("%s%s - (%v instances found)", s.input.Prompt, s.Current.Query, s.timesFound)
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
	s.jqMode = false
	s.Current = SearchQuery{}
	s.applyPrompt()
	s.historyPointer = 0
	return func() tea.Msg {
		return common.SetModalUpdateFuncMsg(s.Update)
	}
}

func (s *Search) CancelModal() tea.Cmd {
	return func() tea.Msg {
		return common.SetModalUpdateFuncMsg(nil)
	}
}

func (s *Search) SetMatchCount(count int) {
	s.timesFound = count
}

func (s *Search) applySearchQuery(query SearchQuery) tea.Cmd {
	return func() tea.Msg {
		return ApplySearchQueryMsg(query)
	}
}
