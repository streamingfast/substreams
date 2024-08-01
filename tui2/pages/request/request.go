package request

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/replaylog"
	streamui "github.com/streamingfast/substreams/tui2/stream"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
)

type NewRequestInstance *Instance

type BlockContext struct {
	Module   string
	BlockNum uint64
}

type Config struct {
	ManifestPath                string
	Pkg                         *pbsubstreams.Package
	Graph                       *manifest.ModuleGraph
	ReadFromModule              bool
	ProdMode                    bool
	DebugModulesOutput          []string
	DebugModulesInitialSnapshot []string
	StartBlock                  int64
	StopBlock                   uint64
	FinalBlocksOnly             bool
	Headers                     map[string]string
	OutputModule                string
	SubstreamsClientConfig      *client.SubstreamsClientConfig
	HomeDir                     string
	Vcr                         bool
	Cursor                      string
	Params                      map[string]string
	ReaderOptions               []manifest.Option
}

type Instance struct {
	Stream         *streamui.Stream
	MsgDescs       map[string]*manifest.ModuleDescriptor
	ReplayLog      *replaylog.File
	RequestSummary *Summary
	Modules        *pbsubstreams.Modules
	Graph          *manifest.ModuleGraph
}

type Request struct {
	common.Common

	form               *huh.Form
	formStartBlock     string
	formStopBlock      string
	formEndpoint       string
	formModuleSelected string

	RequestSummary     *Summary
	Modules            *pbsubstreams.Modules
	manifestView       viewport.Model
	traceId            string
	resolvedStartBlock uint64
	linearHandoffBlock uint64
	parallelWorkers    uint64
	params             map[string][]string
}

func New(c common.Common) *Request {
	return &Request{
		Common:       c,
		manifestView: viewport.New(24, 80),
	}
}

func (r *Request) Init() tea.Cmd {
	r.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("module").
				Value(&r.formModuleSelected).
				Inline(true).
				Options(huh.NewOptions("map_things_events_calls", "graph_out")...).
				Title("Select module to stream:"),
			huh.NewInput().
				Key("start_block").
				Value(&r.formStartBlock).
				Inline(true).
				Validate(func(s string) error {
					if !regexp.MustCompile(`^\d+$`).MatchString(s) {
						return fmt.Errorf("specify only numbers")
					}
					return nil
				}).
				Title("Enter the start block number:"),
			huh.NewInput().
				Key("stop_block").
				Inline(true).
				Value(&r.formStopBlock).
				Validate(func(s string) error {
					if !regexp.MustCompile(`^[\+\-]?\d+$`).MatchString(s) {
						return fmt.Errorf("specify only numbers, optionally prefixed by - or +")
					}
					return nil
				}).
				Title("Stream to block:").
				Description("You can specify relative block numbers with - (to head) or + (to start block) prefixes."),
		),
	)
	return tea.Batch(
		r.manifestView.Init(),
		r.form.Init(),
	)
}

func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	_, cmd := r.form.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case NewRequestInstance:
		r.RequestSummary = msg.RequestSummary
		r.Modules = msg.Modules

		r.params = make(map[string][]string)
		if msg.RequestSummary.Params != nil {
			for k, v := range msg.RequestSummary.Params {
				r.params[k] = append(r.params[k], v)
			}
		}
		r.setModulesViewContent()
	case tea.KeyMsg:
		var cmd tea.Cmd
		r.manifestView, cmd = r.manifestView.Update(msg)
		cmds = append(cmds, cmd)
	case *pbsubstreamsrpc.SessionInit:
		r.traceId = msg.TraceId
		r.resolvedStartBlock = msg.ResolvedStartBlock
		r.linearHandoffBlock = msg.LinearHandoffBlock
		r.parallelWorkers = msg.MaxParallelWorkers
	}
	return r, tea.Batch(cmds...)
}

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)

	summaryHeight := lipgloss.Height(r.renderRequestSummary())
	formHeight := lipgloss.Height(r.renderForm())
	r.manifestView.Height = max(h-summaryHeight-formHeight-2 /* for borders */, 0)
	r.manifestView.Width = w
}

func (r *Request) View() string {
	out := lipgloss.JoinVertical(0,
		r.renderForm(),
		r.renderRequestSummary(),
		r.renderManifestView(),
	)
	//fmt.Println("OUTPUT", lipgloss.Height(out), r.Height)
	return out
}

func (r *Request) renderForm() string {
	return lipgloss.NewStyle().MaxHeight(6).Render(r.form.View())
}

func (r *Request) renderManifestView() string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(r.Width - 2).
		MaxHeight(r.manifestView.Height + 2 /* for borders */).
		Render(
			r.manifestView.View(),
		)
}

func (r *Request) renderRequestSummary() string {
	if r.RequestSummary == nil {
		return ""
	}
	summary := r.RequestSummary
	labels := []string{
		"Package: ",
		"Endpoint: ",
		"Start Block: ",
		"Parameters: ",
		"Production mode: ",
		"Trace ID: ",
		"Parallel Workers: ",
		// TODO: add docs field
	}

	handoffStr := ""
	if r.resolvedStartBlock != r.linearHandoffBlock {
		handoffStr = fmt.Sprintf(" (handoff: %d)", r.linearHandoffBlock)
	}

	paramsStrings := make([]string, 0, len(summary.Params))
	for k, v := range summary.Params {
		paramsStrings = append(paramsStrings, fmt.Sprintf("%s=%s", k, v))
	}

	values := []string{
		summary.Manifest,
		summary.Endpoint,
		fmt.Sprintf("%d%s", r.resolvedStartBlock, handoffStr),
		strings.Join(paramsStrings, ", "),
		fmt.Sprintf("%v", summary.ProductionMode),
		r.traceId,
		fmt.Sprintf("%d", r.parallelWorkers),
	}
	if len(summary.InitialSnapshot) > 0 {
		labels = append(labels, "Initial snapshots: ")
		values = append(values, strings.Join(summary.InitialSnapshot, ", "))
	}

	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(r.Width - 2)

	return style.Render(
		lipgloss.NewStyle().Padding(1, 2, 1, 2).Render(
			lipgloss.JoinHorizontal(
				0.5,
				lipgloss.JoinVertical(0, labels...),
				lipgloss.JoinVertical(0, values...),
			),
		),
	)
}

func (r *Request) setModulesViewContent() {
	content, _ := r.getViewportContent()
	r.manifestView.SetContent(content)
}

func (r *Request) getViewportContent() (string, error) {
	output := ""

	for i, module := range r.Modules.Modules {
		if len(r.RequestSummary.ModuleDocs) < i+1 {
			break
		}
		var moduleDoc string
		var err error

		moduleDoc, err = r.getViewPortDropdown(r.RequestSummary.ModuleDocs[i])
		if err != nil {
			return "", fmt.Errorf("getting module doc: %w", err)
		}

		output += fmt.Sprintf("%s\n\n", module.Name)
		output += fmt.Sprintf("	Initial block: %v\n", module.InitialBlock)
		output += fmt.Sprintln("	Inputs: ")
		for i := range module.Inputs {
			if module.Inputs[i].GetParams() != nil && r.params[module.Name] != nil {
				output += fmt.Sprintf("		- params: [%s]\n", strings.Join(r.params[module.Name], ", "))
			} else {
				output += fmt.Sprintf("		- %s\n", module.Inputs[i])
			}
		}
		output += fmt.Sprintln("	Outputs: ")
		output += fmt.Sprintf("		- %s\n", module.Output)
		output += moduleDoc
		if i <= len(r.Modules.Modules)-1 {
			output += "\n\n"
		}
	}

	return lipgloss.NewStyle().Padding(2, 4, 1, 4).Render(output), nil
}

func (r *Request) getViewPortDropdown(moduleMetadata *pbsubstreams.ModuleMetadata) (string, error) {
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

func (c *Config) NewInstance() (*Instance, error) {
	if c.ReadFromModule {
		sb, err := c.Graph.ModuleInitialBlock(c.OutputModule)
		if err != nil {
			return nil, fmt.Errorf("getting module start block: %w", err)
		}
		c.StartBlock = int64(sb)
	}

	ssClient, _, callOpts, headers, err := client.NewSubstreamsClient(c.SubstreamsClientConfig)
	if err != nil {
		return nil, fmt.Errorf("substreams client setup: %w", err)
	}
	if headers == nil {
		headers = make(map[string]string)
	}

	req := &pbsubstreamsrpc.Request{
		StartBlockNum:                       c.StartBlock,
		StartCursor:                         c.Cursor,
		FinalBlocksOnly:                     c.FinalBlocksOnly,
		StopBlockNum:                        uint64(c.StopBlock),
		Modules:                             c.Pkg.Modules,
		OutputModule:                        c.OutputModule,
		ProductionMode:                      c.ProdMode,
		DebugInitialStoreSnapshotForModules: c.DebugModulesInitialSnapshot,
	}

	c.Headers = headers.Append(c.Headers)
	stream := streamui.New(req, ssClient, c.Headers, callOpts)

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	replayLogFilePath := filepath.Join(c.HomeDir, "replay.log")
	replayLog := replaylog.New(replaylog.WithPath(replayLogFilePath))
	if c.Vcr {
		stream.ReplayBundle, err = replayLog.ReadReplay()
		if err != nil {
			return nil, err
		}
	} else {
		if err := replayLog.OpenForWriting(); err != nil {
			return nil, err
		}
		//defer replayLog.Close()
	}

	debugLogPath := filepath.Join(c.HomeDir, "debug.log")
	tea.LogToFile(debugLogPath, "gui:")

	msgDescs, err := manifest.BuildMessageDescriptors(c.Pkg)
	if err != nil {
		return nil, fmt.Errorf("building message descriptors: %w", err)
	}

	requestSummary := &Summary{
		Manifest:        c.ManifestPath,
		Endpoint:        c.SubstreamsClientConfig.Endpoint(),
		ProductionMode:  c.ProdMode,
		InitialSnapshot: req.DebugInitialStoreSnapshotForModules,
		Docs:            c.Pkg.PackageMeta,
		ModuleDocs:      c.Pkg.ModuleMeta,
		Params:          c.Params,
	}

	substreamRequirements := &Instance{
		Stream:         stream,
		MsgDescs:       msgDescs,
		ReplayLog:      replayLog,
		RequestSummary: requestSummary,
		Modules:        c.Pkg.Modules,
		Graph:          c.Graph,
	}

	return substreamRequirements, nil
}
