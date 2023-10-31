package request

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/replaylog"
	streamui "github.com/streamingfast/substreams/tui2/stream"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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
	Params                      []string
}

type Instance struct {
	Stream         *streamui.Stream
	MsgDescs       map[string]*manifest.ModuleDescriptor
	ReplayLog      *replaylog.File
	RequestSummary *Summary
	Modules        *pbsubstreams.Modules
	RefreshCtx     *Config
	Graph          *manifest.ModuleGraph
}

type Request struct {
	common.Common

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
	return tea.Batch(
		r.manifestView.Init(),
	)
}

func (r *Request) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case NewRequestInstance:
		r.RequestSummary = msg.RequestSummary
		r.Modules = msg.Modules

		r.params = make(map[string][]string)
		if msg.RequestSummary.Params != nil {
			for _, p := range msg.RequestSummary.Params {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					panic("invalid params in gui")
				}
				r.params[kv[0]] = append(r.params[kv[0]], kv[1])
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

func (r *Request) View() string {
	lineCount := r.manifestView.TotalLineCount()
	progress := float64(r.manifestView.YOffset+r.manifestView.Height-1) / float64(lineCount) * 100.0

	requestContent := lipgloss.JoinVertical(0,
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(r.Width-2).Render(r.manifestView.View()),
		lipgloss.NewStyle().MarginLeft(r.Width-len(fmt.Sprint(lineCount))-15).Render(fmt.Sprintf("%.1f%% of %v lines", progress, lineCount)),
	)

	return lipgloss.JoinVertical(0,
		r.renderRequestSummary(),
		requestContent,
	)
}

func (r *Request) renderRequestSummary() string {
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

	values := []string{
		fmt.Sprintf("%s", summary.Manifest),
		fmt.Sprintf("%s", summary.Endpoint),
		fmt.Sprintf("%d%s", r.resolvedStartBlock, handoffStr),
		strings.Join(summary.Params, ", "),
		fmt.Sprintf("%v", summary.ProductionMode),
		r.traceId,
		fmt.Sprintf("%d", r.parallelWorkers),
	}
	if len(summary.InitialSnapshot) > 0 {
		labels = append(labels, "Initial snapshots: ")
		values = append(values, fmt.Sprintf("%s", strings.Join(summary.InitialSnapshot, ", ")))
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

func (r *Request) SetSize(w, h int) {
	r.Common.SetSize(w, h)
	r.manifestView.Width = w
	r.manifestView.Height = h - 16
}

func (r *Request) setModulesViewContent() {
	content, _ := r.getViewportContent()
	r.manifestView.SetContent(content)
}

func (r *Request) getViewportContent() (string, error) {
	output := ""
	for i, module := range r.Modules.Modules {
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
		markdown += "# " + fmt.Sprintf("docs: \n")
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
	graph, pkg, err := readManifest(c.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("graph and package setup: %w", err)
	}
	if c.ReadFromModule {
		sb, err := graph.ModuleInitialBlock(c.OutputModule)
		if err != nil {
			return nil, fmt.Errorf("getting module start block: %w", err)
		}
		c.StartBlock = int64(sb)
	}

	if err := manifest.ApplyParams(c.Params, pkg); err != nil {
		return nil, err
	}

	ssClient, _, callOpts, err := client.NewSubstreamsClient(c.SubstreamsClientConfig)
	if err != nil {
		return nil, fmt.Errorf("substreams client setup: %w", err)
	}
	//defer connClose()

	req := &pbsubstreamsrpc.Request{
		StartBlockNum:                       c.StartBlock,
		StartCursor:                         c.Cursor,
		FinalBlocksOnly:                     c.FinalBlocksOnly,
		StopBlockNum:                        uint64(c.StopBlock),
		Modules:                             pkg.Modules,
		OutputModule:                        c.OutputModule,
		ProductionMode:                      c.ProdMode,
		DebugInitialStoreSnapshotForModules: c.DebugModulesInitialSnapshot,
	}

	stream := streamui.New(req, ssClient, c.Headers, callOpts)

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	toPrint := c.DebugModulesOutput
	if toPrint == nil {
		toPrint = []string{c.OutputModule}
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

	msgDescs, err := manifest.BuildMessageDescriptors(pkg)
	if err != nil {
		return nil, fmt.Errorf("building message descriptors: %w", err)
	}

	requestSummary := &Summary{
		Manifest:        c.ManifestPath,
		Endpoint:        c.SubstreamsClientConfig.Endpoint(),
		ProductionMode:  c.ProdMode,
		InitialSnapshot: req.DebugInitialStoreSnapshotForModules,
		Docs:            pkg.PackageMeta,
		ModuleDocs:      pkg.ModuleMeta,
		Params:          c.Params,
	}

	substreamRequirements := &Instance{
		Stream:         stream,
		MsgDescs:       msgDescs,
		ReplayLog:      replayLog,
		RequestSummary: requestSummary,
		Modules:        pkg.Modules,
		Graph:          graph,
	}

	return substreamRequirements, nil
}

func readManifest(manifestPath string) (*manifest.ModuleGraph, *pbsubstreams.Package, error) {
	manifestReader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("manifest reader: %w", err)
	}

	pkg, err := manifestReader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return nil, nil, fmt.Errorf("creating module graph: %w", err)
	}
	return graph, pkg, nil
}
