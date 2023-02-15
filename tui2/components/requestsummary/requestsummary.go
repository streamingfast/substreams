package requestsummary

import (
	"fmt"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"golang.org/x/term"
)

type RequestSummary struct {
	common.Common

	Manifest        string
	Endpoint        string
	DevMode         bool
	InitialSnapshot string
}

func New(c common.Common, summary *RequestSummary) *RequestSummary {
	return &RequestSummary{
		c,
		summary.Manifest,
		summary.Endpoint,
		summary.DevMode,
		summary.InitialSnapshot,
	}
}

func (r *RequestSummary) Init() tea.Cmd {
	return nil
}

func (r *RequestSummary) View() string {
	labels := []string{
		"Package: ",
		"Endpoint: ",
		"Dev Mode: ",
		"Initial Snapshot: ",
	}
	values := []string{
		fmt.Sprintf("%s", r.Manifest),
		fmt.Sprintf("%s", r.Endpoint),
		fmt.Sprintf("%v", r.DevMode),
		fmt.Sprintf("%s", r.InitialSnapshot),
	}

	var terminalWidth, _, _ = term.GetSize(0)
	vp := viewport.New(terminalWidth, 10)
	vp.Style = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true)
	vp.SetContent(lipgloss.JoinVertical(0,
		lipgloss.NewStyle().Padding(2, 4, 2, 4).Render(lipgloss.JoinHorizontal(0.5,
			lipgloss.JoinVertical(0, labels...),
			lipgloss.JoinVertical(0, values...),
		)),
	))
	return vp.View()
}
