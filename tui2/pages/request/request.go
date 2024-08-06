package request

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/components/dataentry"
	"github.com/streamingfast/substreams/tui2/components/modsearch"
	"github.com/streamingfast/substreams/tui2/replaylog"
	streamui "github.com/streamingfast/substreams/tui2/stream"
	"github.com/streamingfast/substreams/tui2/styles"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

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
	RawStopBlock                string
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
	*Config

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

func New(c common.Common, conf *Config) *Request {
	return &Request{
		Common:       c,
		Config:       conf,
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
			for k, v := range msg.RequestSummary.Params {
				r.params[k] = append(r.params[k], v)
			}
		}
		r.setModulesViewContent()

	case common.SetRequestValue:
		log.Println("Received value", msg.Field, msg.Value)
		switch msg.Field {
		case "start-block":
			r.formStartBlock = msg.Value
		case "stop-block":
			r.formStopBlock = msg.Value
		}
	case common.ModuleSelectedMsg:
		log.Println("Setting selected module:", msg)
		r.Config.OutputModule = string(msg)

	case tea.KeyMsg:
		// COuld we support:
		// `s` to change `start block`
		// `t` to change stop block
		// `m` and `M` module to use module search/module fuzzy search in the `request` tab, and pick from there
		// `p` to change parameters (?)
		// `e` to change endpoint
		// `m` to change manifest path
		// `v` to change vcr mode
		// `h` to change headers
		// `o` to change output module
		// `d` to change debug modules
		// `i` to change debug initial snapshot
		// `f` to change final blocks only
		// `g` to change read from module
		// `b` to change home dir
		// `w` to change substreams client config
		// `z` to change reader options
		// `a` for advanced options (shows Final Blocks Only switcher)
		// `c` to change cursor

		switch msg.String() {
		case "s":
			comp := dataentry.New(r.Common, "start-block", validateNumbersOnly)
			comp.Input.Prompt("Enter the start block number: ").
				Description("Block from which to start streaming. Numbers only\n\n")
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "t":
			comp := dataentry.New(r.Common, "stop-block", validateNumberOrRelativeValue)
			comp.Input.Prompt("Enter the stop block number: ").
				Description("Enter numbers only, with an optional - or + prefix.\n\nYou can specify relative block numbers with - (to head) or + (to start block) prefixes.\n")
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "m":
			comp := modsearch.New(r.Common)
			comp.SetListItems(r.Config.Graph.Modules())
			cmds = append(cmds, common.SetModalComponentCmd(comp))
		case "e":
		case "a":

		default:
			var cmd tea.Cmd
			r.manifestView, cmd = r.manifestView.Update(msg)
			cmds = append(cmds, cmd)
		}

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
	r.manifestView.Height = max(h-summaryHeight-2 /* for borders */, 0)
	r.manifestView.Width = w
}

func (r *Request) View() string {
	out := lipgloss.JoinVertical(0,
		r.renderRequestSummary(),
		r.renderManifestView(),
	)
	//fmt.Println("OUTPUT", lipgloss.Height(out), r.Height)
	return out
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

	handoffStr := ""
	if r.resolvedStartBlock != r.linearHandoffBlock {
		handoffStr = fmt.Sprintf(" (handoff: %d)", r.linearHandoffBlock)
	}
	summary := r.RequestSummary
	paramsStrings := make([]string, 0, len(summary.Params))
	for k, v := range summary.Params {
		paramsStrings = append(paramsStrings, fmt.Sprintf("%s=%s", k, v))
	}

	startBlock := fmt.Sprintf("%d%s", r.resolvedStartBlock, handoffStr)
	startBlock = r.formStartBlock

	rows := [][]string{
		{"Package:", summary.Manifest},
		{"[m] Module:", r.Config.OutputModule},
		{"[e] Endpoint:", summary.Endpoint},
		{"[s] Start Block:", startBlock},
		{"[t] Stop Block:", r.Config.RawStopBlock},
		{"Parameters:", fmt.Sprintf("%v", summary.Params)},
		{"Production mode:", fmt.Sprintf("%v", summary.ProductionMode)},
		{"Trace ID:", r.traceId},
		{"Parallel Workers:", fmt.Sprintf("%d", r.parallelWorkers)},
	}

	if len(summary.InitialSnapshot) > 0 {
		rows = append(rows, []string{"Initial snapshots:", strings.Join(summary.InitialSnapshot, ", ")})
	}

	t := table.New().Border(lipgloss.Border{}).Width(r.Width - 2).Rows(rows...).StyleFunc(func(row, col int) lipgloss.Style {
		color := styles.RequestOddRow
		if row%2 == 0 {
			color = styles.RequestEvenRow
		}
		if col == 0 {
			return color.Align(lipgloss.Right)
		}
		return color
	})

	return t.Render()
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
