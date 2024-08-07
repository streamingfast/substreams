package docs

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/pages/request"
)

type Docs struct {
	common.Common

	docsView viewport.Model

	reqSummary *request.Summary
	modules    *pbsubstreams.Modules
	params     map[string][]string
}

func New(c common.Common) *Docs {
	page := &Docs{
		Common:   c,
		docsView: viewport.New(c.Width, c.Height),
		params:   make(map[string][]string),
	}
	return page
}

func (d *Docs) setNewRequest(reqSummary *request.Summary, modules *pbsubstreams.Modules) {
	d.reqSummary = reqSummary
	d.modules = modules

	d.params = make(map[string][]string)
	if reqSummary.Params != nil {
		for k, v := range reqSummary.Params {
			d.params[k] = append(d.params[k], v)
		}
	}
	d.setModulesViewContent()
}

func (d *Docs) Init() tea.Cmd {
	return d.docsView.Init()
}

func (d *Docs) SetSize(w, h int) {
	d.Common.SetSize(w, h)
	d.docsView.Height = max(h-2 /* for borders */, 0)
	d.docsView.Width = w
}

func (d *Docs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case string:
		log.Println("message bob", msg)
	case request.NewRequestInstance:
		d.setNewRequest(msg.RequestSummary, msg.Modules)
	}
	var cmd tea.Cmd
	d.docsView, cmd = d.docsView.Update(msg)
	return d, cmd
}

func (d *Docs) View() string {
	return d.renderManifestView()
}

func (d *Docs) setModulesViewContent() {
	content, _ := d.getViewportContent()
	d.docsView.SetContent(content)
}

func (d *Docs) renderManifestView() string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(d.Width - 2).
		MaxHeight(d.docsView.Height + 2 /* for borders */).
		Render(
			d.docsView.View(),
		)
}

func (d *Docs) getViewportContent() (string, error) {
	output := ""

	for i, module := range d.modules.Modules {
		if len(d.reqSummary.ModuleDocs) < i+1 {
			break
		}
		var moduleDoc string
		var err error

		moduleDoc, err = d.getViewPortDropdown(d.reqSummary.ModuleDocs[i])
		if err != nil {
			return "", fmt.Errorf("getting module doc: %w", err)
		}

		output += fmt.Sprintf("%s\n\n", module.Name)
		output += fmt.Sprintf("	Initial block: %v\n", module.InitialBlock)
		output += fmt.Sprintln("	Inputs: ")
		for i, input := range module.Inputs {
			_ = input
			// switch input.(type) {

			// }
			// switch module.Inputs[i].(type) {
			// case *pbsubstreams.ModuleInputBlock:
			// case *pbsubstreams.ModuleInputCursor:
			// case *pbsubstreams.ModuleInputParams:
			// case *pbsubstreams.ModuleInputParams:
			// }
			if module.Inputs[i].GetParams() != nil && d.params[module.Name] != nil {
				output += fmt.Sprintf("		- params: [%s]\n", strings.Join(d.params[module.Name], ", "))
			} else {
				output += fmt.Sprintf("		- %s\n", module.Inputs[i])
			}
		}
		output += fmt.Sprintln("	Outputs: ")
		output += fmt.Sprintf("		- %s\n", module.Output)
		output += moduleDoc
		if i <= len(d.modules.Modules)-1 {
			output += "\n\n"
		}
	}

	return lipgloss.NewStyle().Padding(2, 4, 1, 4).Render(output), nil
}

func (d *Docs) getViewPortDropdown(moduleMetadata *pbsubstreams.ModuleMetadata) (string, error) {
	content, err := glamorizeDoc(moduleMetadata.GetDoc())
	if err != nil {
		return "", fmt.Errorf("getting module docs: %w", err)
	}

	return content, nil
}

func glamorizeDoc(doc string) (string, error) {
	markdown := ""

	if doc != "" {
		markdown += "# " + "docs: \n"
		markdown += "\n"
		markdown += doc
		markdown += "\n"
	}
	markdown += "\n\n"

	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}
	out, err := glamour.Render(markdown, style)
	if err != nil {
		return "", fmt.Errorf("GlamouriseItem: %w", err)
	}

	return out, nil
}
