package request

import (
	"fmt"
	"path/filepath"
	"strconv"
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

type NewRequestInstance *RequestInstance

type BlockContext struct {
	Module   string
	BlockNum uint64
}

type RequestConfig struct {
	ManifestPath                string
	ReadFromModule              bool
	ProdMode                    bool
	DebugModulesOutput          []string
	DebugModulesInitialSnapshot []string
	StartBlock                  int64
	StopBlock                   string
	FinalBlocksOnly             bool
	OutputModule                string
	SubstreamsClientConfig      *client.SubstreamsClientConfig
	HomeDir                     string
	Vcr                         bool
	Cursor                      string
}

type RequestInstance struct {
	Stream         *streamui.Stream
	MsgDescs       map[string]*manifest.ModuleDescriptor
	ReplayLog      *replaylog.File
	RequestSummary *Summary
	Modules        *pbsubstreams.Modules
	RefreshCtx     *RequestConfig
	Graph          *manifest.ModuleGraph
}

type Request struct {
	common.Common

	RequestSummary     *Summary
	Modules            *pbsubstreams.Modules
	manifestView       viewport.Model
	modulesViewContent string
	traceId            string
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
		r.setModulesViewContent()
	case tea.KeyMsg:
		var cmd tea.Cmd
		r.manifestView, cmd = r.manifestView.Update(msg)
		cmds = append(cmds, cmd)
	case *pbsubstreamsrpc.SessionInit:
		r.traceId = msg.TraceId
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
		"Production mode: ",
		"Initial snapshots: ",
		"Trace ID: ",
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
	values = append(values, fmt.Sprintf("%s", r.traceId))
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
	r.manifestView.Width = w
	r.manifestView.Height = h - 11
}

func (r *Request) setModulesViewContent() {
	content, _ := r.getViewportContent()
	r.modulesViewContent = content
	r.manifestView.SetContent(content)
}

func (r *Request) getViewportContent() (string, error) {
	output := ""
	for i, module := range r.Modules.Modules {

		var moduleDoc string

		var err error
		if i <= len(r.RequestSummary.Docs)-1 {
			moduleDoc, err = r.getViewPortDropdown(r.RequestSummary.Docs[i], module)
			if err != nil {
				return "", fmt.Errorf("getting module doc: %w", err)
			}
		}

		output += fmt.Sprintf("%s\n\n", module.Name)
		output += fmt.Sprintf("	Initial block: %v\n", module.InitialBlock)
		output += fmt.Sprintln("	Inputs: ")
		for i := range module.Inputs {
			output += fmt.Sprintf("		- %s\n", module.Inputs[i])
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

func (r *Request) getViewPortDropdown(metadata *pbsubstreams.PackageMetadata, module *pbsubstreams.Module) (string, error) {
	content, err := glamouriseModuleDoc(metadata, module)
	if err != nil {
		return "", fmt.Errorf("getting module docs: %w", err)
	}

	return content, nil
}

func glamouriseModuleDoc(metadata *pbsubstreams.PackageMetadata, module *pbsubstreams.Module) (string, error) {
	markdown := ""

	markdown += "# " + fmt.Sprintf("docs: \n")
	markdown += "\n"
	if metadata.GetDoc() != "" {
		markdown += "[doc]: " + "" + metadata.GetDoc()
		markdown += "\n"
	}
	markdown += "\n\n"

	out, err := glamour.Render(markdown, "dark")
	if err != nil {
		return "", fmt.Errorf("GlamouriseItem: %w", err)
	}

	return out, nil
}

func (c *RequestConfig) NewInstance() (*RequestInstance, error) {
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

	stopBlock, err := resolveStopBlock(c.StopBlock, c.StartBlock)
	if err != nil {
		return nil, fmt.Errorf("stop block: %w", err)
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
		StopBlockNum:                        stopBlock,
		Modules:                             pkg.Modules,
		OutputModule:                        c.OutputModule,
		ProductionMode:                      c.ProdMode,
		DebugInitialStoreSnapshotForModules: c.DebugModulesInitialSnapshot,
	}

	stream := streamui.New(req, ssClient, callOpts)

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
	}

	substreamRequirements := &RequestInstance{
		stream,
		msgDescs,
		replayLog,
		requestSummary,
		pkg.Modules,
		c,
		graph,
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

func resolveStopBlock(stopBlock string, startBlock int64) (uint64, error) {
	isRelative := strings.HasPrefix(stopBlock, "+")
	if isRelative {
		stopBlock = strings.TrimPrefix(stopBlock, "+")
		if startBlock < 0 {
			return 0, fmt.Errorf("cannot have start block negative with relative stop block")
		}
	}

	endBlock, err := strconv.ParseUint(stopBlock, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("end block is invalid: %w", err)
	}

	if isRelative {
		return uint64(startBlock) + endBlock, nil
	}

	return endBlock, nil
}
