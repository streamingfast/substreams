package request

import (
	"fmt"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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
	viewportContent, _ := r.getViewportContent()
	lineCount := strings.Count(viewportContent, "\n")
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
	r.requestView.Height = h - 11
}

func (r *Request) setViewportContent() {
	content, _ := r.getViewportContent()
	//content += fmt.Sprintf("\nParams: %s", strings.Join(r.requestSummary.Params, ","))
	r.requestView.SetContent(content)
}

func (r *Request) getViewportContent() (string, error) {
	output := ""
	paramsMapped := make(map[string][]string)

	// build mapped params
	for _, param := range r.requestSummary.Params {
		paramSplit := strings.Split(param, "=")
		paramsMapped[paramSplit[0]] = append(paramsMapped[paramSplit[0]], paramSplit[1])
	}

	for i, module := range r.modules.Modules {
		curParams := paramsMapped[module.Name]

		var moduleDoc string
		var curParam string
		if curParams == nil {
			curParam = ""
		}
		curParam = curParams[len(curParams)-1]

		moduleDoc, err := r.getViewPortDropdown(r.requestSummary.Docs[i], curParam, module)
		if err != nil {
			return "", fmt.Errorf("getting module doc: %w", err)
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

func (r *Request) getViewPortDropdown(metadata *pbsubstreams.PackageMetadata, param string, module *pbsubstreams.Module) (string, error) {
	content, err := glamouriseModuleDoc(metadata, param, module)
	if err != nil {
		return "", fmt.Errorf("getting module docs: %w", err)
	}

	return content, nil
}

func glamouriseModuleDoc(metadata *pbsubstreams.PackageMetadata, param string, module *pbsubstreams.Module) (string, error) {
	var markdown string

	markdown += "# " + fmt.Sprintf("%s - docs: ", module.Name)
	markdown += "\n"
	markdown += "	[doc]: " + "	" + metadata.GetDoc()
	markdown += "\n"
	if metadata.Url != "" {
		markdown += "	[url]: " + "	" + metadata.Url
		markdown += "\n\n"
	}
	markdown += "	[version]: " + "	" + metadata.Version
	markdown += "\n\n"
	if param != "" {
		markdown += "	[param]: " + "	" + param
		markdown += "\n\n"
	}

	out, err := glamour.Render(markdown, "dark")
	if err != nil {
		return "", fmt.Errorf("GlamouriseItem: %w", err)
	}

	return out, nil
}
