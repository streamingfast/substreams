package request

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/requestsummary"
)

type Request struct {
	common.Common
	KeyMap KeyMap

	requestSummary *requestsummary.RequestSummary
}

func New(c common.Common, summary *requestsummary.RequestSummary) *Request {
	return &Request{
		Common:         c,
		requestSummary: requestsummary.New(c, summary),
	}
}

func (r *Request) Init() tea.Cmd { return nil }
func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return r, nil
}

func (r *Request) View() string {
	return lipgloss.JoinVertical(0,
		r.requestSummary.View(),
	)
}

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)
	r.requestSummary.SetSize(w, 8)
}
