package blocksearch

import (
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/tui2/components/blockselect"
	"github.com/streamingfast/substreams/tui2/components/search"
	"github.com/streamingfast/substreams/tui2/keymap"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/streamingfast/substreams/tui2/common"
)

type UpdateMatchingBlocks map[uint64]bool
type ApplyBlockSearchQueryMsg SearchQuery

type SearchQuery string

type BlockSearch struct {
	common.Common

	input textinput.Model

	History        []string
	historyPointer int
	Current        string

	timesFound int
}

func New(c common.Common) *BlockSearch {
	input := textinput.New()
	input.Placeholder = ""
	input.CharLimit = 1024
	input.Width = c.Width
	input.Prompt = "Go to block: "
	return &BlockSearch{
		Common: c,
		input:  input,
	}
}

func (s *BlockSearch) SetSize(w, h int) {
	s.Common.SetSize(w, h)
	s.input.Width = w
}
func (s *BlockSearch) Init() tea.Cmd {
	s.input.Focus()
	s.input.SetValue("")
	s.Current = ""
	s.historyPointer = 0
	return nil
}

func (s *BlockSearch) ShortHelp() []key.Binding {
	return []key.Binding{
		keymap.GeneralSearchEnter,
		keymap.EscapeModal,
	}
}
func (s *BlockSearch) FullHelp() [][]key.Binding { return common.ShortToFullHelp(s) }

func (s *BlockSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	// TODO: change into a simple `huh` form, in a Modal dialog, take the output and dispatch the message.

	var cmds []tea.Cmd
msgSwitch:
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !s.input.Focused() {
			break
		}
		switch msg.String() {
		case "enter":
			if s.input.Value() == "" {
				s.input.Blur()
				cmds = append(cmds, common.CancelModalCmd())
			} else {
				newQuery := s.input.Value()
				s.Current = newQuery
				s.History = append(s.History, newQuery)
				uintQuery, err := s.CheckValidQuery()
				if err != nil {
					break
				}
				s.input.Blur()
				cmds = append(cmds, func() tea.Msg { return blockselect.BlockChangedMsg(uintQuery) })
				cmds = append(cmds, common.CancelModalCmd())
				break msgSwitch
			}
		case "esc":
			cmds = append(cmds, tea.Sequence(s.clearSearch, common.CancelModalCmd()))
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return s, tea.Batch(cmds...)
}

func (s *BlockSearch) View() string {
	return s.input.View()
}

func (s *BlockSearch) clearSearch() tea.Msg {
	return search.SearchClearedMsg(true)
}

func (s *BlockSearch) SetMatchCount(count int) {
	s.timesFound = count
}

func (s *BlockSearch) CheckValidQuery() (uint64, error) {
	strippedQuery := strings.ReplaceAll(s.Current, ",", "")
	strippedQuery = strings.ReplaceAll(strippedQuery, "#", "")
	uintQuery, err := strconv.ParseUint(strippedQuery, 10, 64)
	if err != nil {
		return 0, err
	}
	return uintQuery, nil
}
