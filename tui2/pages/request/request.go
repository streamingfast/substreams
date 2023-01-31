package request

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

type Request struct {
	common common.Common
	KeyMap KeyMap
}

func New(c common.Common) *Request {
	return &Request{common: c}
}

func (r *Request) Init() tea.Cmd { return nil }
func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return r, nil
}

func (r *Request) View() string {
	return "request view"
}

func (r *Request) SetSize(width, height int) {
	r.common.SetSize(width, height)
}
