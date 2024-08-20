package build

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/pages/request"
	"github.com/streamingfast/substreams/tui2/styles"
)

type Build struct {
	common.Common

	docsView viewport.Model

	reqSummary *request.Summary
	modules    *pbsubstreams.Modules
	graph      *manifest.ModuleGraph
	hashes     *manifest.ModuleHashes
	params     map[string][]string
}

func New(c common.Common) *Build {
	page := &Build{
		Common:   c,
		docsView: viewport.New(c.Width, c.Height),
		params:   make(map[string][]string),
	}
	return page
}

func (d *Build) setNewRequest(reqSummary *request.Summary, modules *pbsubstreams.Modules, graph *manifest.ModuleGraph) {
	d.reqSummary = reqSummary
	d.modules = modules
	d.graph = graph

	d.hashes = manifest.NewModuleHashes()
	for _, mod := range modules.Modules {
		d.hashes.HashModule(modules, mod, graph)
	}

	d.params = make(map[string][]string)
	if reqSummary.Params != nil {
		for k, v := range reqSummary.Params {
			d.params[k] = append(d.params[k], v)
		}
	}
	d.setModulesViewContent()
}

func (d *Build) Init() tea.Cmd {
	return d.docsView.Init()
}

func (d *Build) SetSize(w, h int) {
	d.Common.SetSize(w, h)
	d.docsView.Height = max(h-2 /* for borders */, 0)
	d.docsView.Width = w
}

func (d *Build) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case string:
		log.Println("message bob", msg)
	case request.NewRequestInstance:
		d.setNewRequest(msg.RequestSummary, msg.Modules, msg.Graph)
	}
	var cmd tea.Cmd
	d.docsView, cmd = d.docsView.Update(msg)
	return d, cmd
}

func (d *Build) View() string {
	return d.renderManifestView()
}

func (d *Build) setModulesViewContent() {
	content, _ := d.getViewportContent()
	d.docsView.SetContent(content)
}

func (d *Build) renderManifestView() string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(d.Width - 2).
		MaxHeight(d.docsView.Height + 2 /* for borders */).
		Render(
			d.docsView.View(),
		)
}

func (d *Build) getViewportContent() (string, error) {
	var lines []string

	lines = append(lines,
		lipgloss.PlaceHorizontal(
			d.Width-styles.DocBox.GetHorizontalFrameSize(), lipgloss.Center,
			" MODULES ", lipgloss.WithWhitespaceChars("-"),
		),
		"",
	)

	for i, module := range d.modules.Modules {
		if len(d.reqSummary.ModuleDocs) < i+1 {
			break
		}
		moduleDoc, err := glamorizeDoc(d.reqSummary.ModuleDocs[i].Doc)
		if err != nil {
			return "", fmt.Errorf("rendering module %q doc: %w", module.Name, err)
		}

		moduleName := module.Name
		switch module.Kind.(type) {
		case *pbsubstreams.Module_KindBlockIndex_:
			moduleName = moduleName + " [block index]"
		case *pbsubstreams.Module_KindStore_:
			moduleName = moduleName + " [store]"
		case *pbsubstreams.Module_KindMap_:
			moduleName = moduleName + " [map]"
		}

		lines = append(lines, styles.DocModuleName.Render(moduleName), "")
		lines = append(lines, fmt.Sprintf("  • Module hash: %s", d.hashes.Get(module.Name)))
		if kind, ok := module.Kind.(*pbsubstreams.Module_KindStore_); ok {
			lines = append(lines, fmt.Sprintf("  • Store policy: %s %s", kind.KindStore.UpdatePolicy.Pretty(), kind.KindStore.ValueType))
		}
		lines = append(lines, fmt.Sprintf("  • Initial block: %v", module.InitialBlock))
		lines = append(lines, "  • Inputs: ")
		for _, input := range module.Inputs {
			switch input := input.Input.(type) {
			case *pbsubstreams.Module_Input_Source_:
				lines = append(lines, fmt.Sprintf("    - source: %s", input.Source.Type))
			case *pbsubstreams.Module_Input_Map_:
				lines = append(lines, fmt.Sprintf("    - map: %s", input.Map.ModuleName))
			case *pbsubstreams.Module_Input_Store_:
				lines = append(lines, fmt.Sprintf("    - store: %s (mode: %s)", input.Store.ModuleName, input.Store.Mode.Pretty()))
			case *pbsubstreams.Module_Input_Params_:
				//lines = append(lines, fmt.Sprintf("    - params: %s"), input.Params.Value)
				lines = append(lines, fmt.Sprintf("    - params: %s", strings.Join(d.params[module.Name], ", ")))
			}
		}
		if module.Output != nil {
			lines = append(lines, "  • Outputs: "+module.Output.Type)
		}
		lines = append(lines, moduleDoc)
	}

	lines = append(lines,
		lipgloss.PlaceHorizontal(
			d.Width-styles.DocBox.GetHorizontalFrameSize(), lipgloss.Center,
			" BUILD OUTPUT ", lipgloss.WithWhitespaceChars("-"),
		),
		"",
	)

	return styles.DocBox.Render(strings.Join(lines, "\n")), nil
}

func glamorizeDoc(doc string) (string, error) {
	markdown := ""

	if doc != "" {
		markdown += doc
		markdown += "\n"
	}
	markdown += "\n"

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
