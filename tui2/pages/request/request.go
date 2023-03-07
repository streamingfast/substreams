package request

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
)

type Request struct {
	common.Common

	requestSummary *Summary
	modules        *pbsubstreams.Modules
	requestView    viewport.Model
}

func New(c common.Common, summary *Summary, modules *pbsubstreams.Modules) *Request {
	return &Request{
		Common:         c,
		requestSummary: summary,
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
	viewportContent, _ := r.getViewportContent()
	lineCount := strings.Count(viewportContent, "\n")
	progress := float64(r.requestView.YOffset+r.requestView.Height-1) / float64(lineCount) * 100.0
	return lipgloss.JoinVertical(0,
		r.renderRequestSummary(),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(r.Width-2).Render(r.requestView.View()),
		lipgloss.NewStyle().MarginLeft(r.Width-len(fmt.Sprint(lineCount))-15).Render(fmt.Sprintf("%.1f%% of %v lines", progress, lineCount)),
	)
}

func (r *Request) renderRequestSummary() string {
	summary := r.requestSummary
	labels := []string{
		"Package: ",
		"Endpoint: ",
		"Production mode: ",
		"Initial snapshots: ",
	}
	values := []string{
		fmt.Sprintf("%s", summary.Manifest),
		fmt.Sprintf("%s", summary.Endpoint),
		fmt.Sprintf("%v", summary.ProductionMode),
	}
	if len(summary.InitialSnapshot) > 0 {
		values = append(values, fmt.Sprintf("%s", strings.Join(summary.InitialSnapshot, ", ")))
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

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)
	r.requestView.Width = w
	r.requestView.Height = h - 11
}

func (r *Request) setViewportContent() {
	content, _ := r.getViewportContent()
	r.requestView.SetContent(content)
}

func (r *Request) getViewportContent() (string, error) {
	output := ""

	for i, module := range r.modules.Modules {

		var moduleDoc string

		var err error
		if i <= len(r.requestSummary.Docs)-1 {
			moduleDoc, err = r.getViewPortDropdown(r.requestSummary.Docs[i], module)
			if err != nil {
				return "", fmt.Errorf("getting module doc: %w", err)
			}
		}

		output += fmt.Sprintf("%s\n\n", module.Name)
		output += fmt.Sprintf("	Initial block: %v\n", module.InitialBlock)
		output += fmt.Sprintln("	Inputs: ")
		for i, _ := range module.Inputs {
			output += fmt.Sprintf("		- %s\n", module.Inputs[i])
		}
		output += fmt.Sprintln("	Outputs: ")
		output += fmt.Sprintf("		%s\n", module.Output)
		output += moduleDoc
		if i <= len(r.modules.Modules)-1 {
			output += "\n\n"
		}
	}

	return lipgloss.NewStyle().Padding(2, 4, 1, 4).Render(output), nil
}

func (r *Request) getViewPortDropdown(metadata *pbsubstreams.PackageMetadata, module *pbsubstreams.Module) (string, error) {
	content, err := glamouriseModuleDoc(metadata, module)
	if err != nil {
		return "", fmt.Errorf("getting module docs: %w", err)
	}

	return content, nil
}

func glamouriseModuleDoc(metadata *pbsubstreams.PackageMetadata, module *pbsubstreams.Module) (string, error) {
	markdown := ""

	markdown += "# " + fmt.Sprintf("%s - docs: ", module.Name)
	markdown += "\n"
	if metadata.GetDoc() != "" {
		markdown += "	[doc]: " + "	" + metadata.GetDoc()
		markdown += "\n"
	}
	if metadata.Url != "" {
		markdown += "	[url]: " + "	" + metadata.Url
		markdown += "\n\n"
	}
	markdown += "	[version]: " + "	" + metadata.Version
	markdown += "\n\n"

	out, err := glamour.Render(markdown, "dark")
	if err != nil {
		return "", fmt.Errorf("GlamouriseItem: %w", err)
	}

	return out, nil
}
