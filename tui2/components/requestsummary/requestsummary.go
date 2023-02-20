package requestsummary

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
	"strings"
)

type RequestSummary struct {
	common.Common

	Manifest        string
	Endpoint        string
	DevMode         bool
	InitialSnapshot []string
	Docs            []*pbsubstreams.PackageMetadata
	Params          []string
}

func New(c common.Common, summary *RequestSummary) *RequestSummary {
	return &RequestSummary{
		c,
		summary.Manifest,
		summary.Endpoint,
		summary.DevMode,
		summary.InitialSnapshot,
		summary.Docs,
		summary.Params,
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
	}
	if len(r.InitialSnapshot) > 0 {
		values = append(values, fmt.Sprintf("%s", strings.Join(r.InitialSnapshot, ", ")))
	} else {
		values = append(values, r.Styles.StatusBarValue.Render(fmt.Sprintf("None")))
	}

	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(r.Width - 2)

	return style.Render(
		lipgloss.NewStyle().Padding(1, 2, 1, 2).Render(lipgloss.JoinHorizontal(0.5,
			lipgloss.JoinVertical(0, labels...),
			lipgloss.JoinVertical(0, values...),
		)),
	)
}
