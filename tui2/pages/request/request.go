package request

import (
	"fmt"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/requestsummary"
	"strings"
)

type Request struct {
	common.Common

	requestSummary *requestsummary.RequestSummary
	modules        *pbsubstreams.Modules
	requestView    viewport.Model
}

func New(c common.Common, summary *requestsummary.RequestSummary, modules *pbsubstreams.Modules) *Request {
	return &Request{
		Common:         c,
		requestSummary: requestsummary.New(c, summary),
		modules:        modules,
		requestView:    viewport.New(24, 80),
	}
}

func (r *Request) Init() tea.Cmd {
	return tea.Batch(
		r.requestView.Init(),
	)
}
func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	r.setViewportContent()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		var cmd tea.Cmd
		r.requestView, cmd = r.requestView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return r, tea.Batch(cmds...)
}

func (r *Request) View() string {
	lineCount := strings.Count(r.getViewportContent(), "\n")
	return lipgloss.JoinVertical(0,
		r.requestSummary.View(),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(r.Width-2).Render(r.requestView.View()),
		lipgloss.NewStyle().MarginLeft(r.Width-len(string(lineCount))-15).Render(fmt.Sprintf("Total lines: %v", lineCount)),
	)
}

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)
	r.requestSummary.SetSize(w, 8)
	r.requestView.Width = w
	r.requestView.Height = h - 9
}

func (r *Request) setViewportContent() {
	r.requestView.SetContent(r.getViewportContent())
}

func (r *Request) getViewportContent() string {
	output := ""
	for i, module := range r.modules.Modules {
		output += fmt.Sprintf("%s\n\n", module.Name)
		output += fmt.Sprintf("	Initial block: %v\n", module.InitialBlock)
		output += fmt.Sprintln("	Inputs: ")
		for i, _ := range module.Inputs {
			output += fmt.Sprintf("		- %s\n", module.Inputs[i])
		}
		output += fmt.Sprintln("	Outputs: ")
		output += fmt.Sprintf("		%s\n", module.Output)
		if i <= len(r.modules.Modules)-1 {
			output += "\n\n"
		}
	}

	return lipgloss.NewStyle().Padding(2, 4, 1, 4).Render(output)
}
