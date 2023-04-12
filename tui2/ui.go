package tui2

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/replaylog"
	"log"
	"path/filepath"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/footer"
	"github.com/streamingfast/substreams/tui2/pages/output"
	"github.com/streamingfast/substreams/tui2/pages/progress"
	"github.com/streamingfast/substreams/tui2/pages/request"
	streamui "github.com/streamingfast/substreams/tui2/stream"
	"github.com/streamingfast/substreams/tui2/styles"
	"github.com/streamingfast/substreams/tui2/tabs"
)

type page int

const (
	requestPage page = iota
	progressPage
	outputPage
)

type UI struct {
	msgDescs             map[string]*manifest.ModuleDescriptor
	stream               *streamui.Stream
	replayLog            *replaylog.File
	originalSubstreamCtx *request.OriginalSubstreamContext // all boilerplate to pass down to refresh

	common.Common
	pages      []common.Component
	activePage page
	footer     *footer.Footer
	showFooter bool
	error      error
	tabs       *tabs.Tabs
}

func New(stream *streamui.Stream, msgDescs map[string]*manifest.ModuleDescriptor, vcr *replaylog.File, reqSummary *request.Summary, modules *pbsubstreams.Modules, refreshCtx request.OriginalSubstreamContext) *UI {
	c := common.Common{
		Styles: styles.DefaultStyles(),
	}
	ui := &UI{
		msgDescs: msgDescs,
		stream:   stream,
		Common:   c,
		pages: []common.Component{
			request.New(c, reqSummary, modules),
			progress.New(c, stream.TargetParallelProcessingBlock()),
			output.New(c, msgDescs, modules),
		},
		activePage:           progressPage,
		tabs:                 tabs.New(c, []string{"Request", "Progress", "Output"}),
		replayLog:            vcr,
		originalSubstreamCtx: &refreshCtx,
	}
	ui.footer = footer.New(c, ui.pages[0])

	return ui
}

func (ui *UI) Init() tea.Cmd {
	var cmds []tea.Cmd

	for _, pg := range ui.pages {
		cmds = append(cmds, pg.Init())
	}

	cmds = append(cmds,
		ui.tabs.Init(),
		ui.footer.Init(),
	)

	cmds = append(cmds, tabs.SelectTabCmd(1))
	cmds = append(cmds, ui.stream.Init())

	if ui.replayLog.IsWriting() {
		cmds = append(cmds, func() tea.Msg {
			return ui.replayLog
		})
	}

	return tea.Batch(cmds...)
}

func (ui *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if bundle, ok := msg.(streamui.ReplayBundle); ok {
		for _, el := range bundle {
			_, _ = ui.update(el)
		}
	}
	if err := ui.replayLog.Push(msg); err != nil {
		log.Printf("Failed to push to replay log: %s", err.Error())
		return ui, tea.Quit
	}
	return ui.update(msg)
}
func (ui *UI) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		ui.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return ui, tea.Quit
		case "R":
			cmd := ui.RefreshSubstream()
			cmds = append(cmds, cmd)
		}
		_, cmd := ui.tabs.Update(msg)
		cmds = append(cmds, cmd)
		_, cmd = ui.pages[ui.activePage].Update(msg)
		cmds = append(cmds, cmd)
		return ui, tea.Batch(cmds...)

	case streamui.Msg:
		switch msg {
		case streamui.ConnectingMsg:
		case streamui.EndOfStreamMsg:
		case streamui.InterruptStreamMsg:

		}
	case tabs.SelectTabMsg:
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
		ui.SetSize(ui.Width, ui.Height)
	case tabs.ActiveTabMsg:
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
		ui.SetSize(ui.Width, ui.Height) // For when the footer changes size here
	}

	cmds = append(cmds, ui.stream.Update(msg))

	_, cmd := ui.footer.Update(msg)
	cmds = append(cmds, cmd)
	_, cmd = ui.tabs.Update(msg)
	cmds = append(cmds, cmd)
	for _, pg := range ui.pages {
		if _, cmd = pg.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return ui, tea.Batch(cmds...)
}

func (ui *UI) SetSize(w, h int) {
	ui.Common.SetSize(w, h)
	footerHeight := ui.footer.Height()
	ui.footer.SetSize(w, footerHeight)
	tabsHeight := ui.tabs.Height
	ui.tabs.SetSize(w, tabsHeight)
	headerHeight := 3
	for _, pg := range ui.pages {
		pg.SetSize(w, h-headerHeight-tabsHeight-footerHeight)
	}
}

func (ui *UI) View() string {
	//ioutil.WriteFile("/tmp/mama.txt", []byte(fmt.Sprintf("MAMA %s\n", ui.common.Styles)), 0644)
	return lipgloss.JoinVertical(0,
		ui.Styles.Header.Copy().Foreground(lipgloss.Color(ui.stream.StreamColor())).Render("Substreams GUI"),
		ui.Styles.Tabs.Render(ui.tabs.View()),
		ui.pages[ui.activePage].View(),
		ui.footer.View(),
	)
}

func (ui *UI) RefreshSubstream() tea.Cmd {
	return func() tea.Msg {
		return request.RefreshSubstream(ui.originalSubstreamCtx)
	}
}

func StartSubstream(ctx *request.OriginalSubstreamContext) (*streamui.Stream, map[string]*manifest.ModuleDescriptor, *replaylog.File, *request.Summary, *pbsubstreams.Modules, *request.OriginalSubstreamContext, *manifest.ModuleGraph, error) {
	graph, pkg, err := GetGraph(ctx.ManifestPath)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("graph and package setup: %w", err)
	}
	ssClient, _, callOpts, err := client.NewSubstreamsClient(ctx.SubstreamsClientConfig)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("substreams client setup: %w", err)
	}
	//defer connClose()

	req := &pbsubstreams.Request{
		StartBlockNum:                       ctx.StartBlock,
		StartCursor:                         ctx.Cursor,
		StopBlockNum:                        ctx.StopBlock,
		ForkSteps:                           []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		Modules:                             pkg.Modules,
		OutputModule:                        ctx.OutputModule,
		OutputModules:                       []string{ctx.OutputModule}, //added for backwards compatibility, will be removed
		ProductionMode:                      ctx.ProdMode,
		DebugInitialStoreSnapshotForModules: ctx.DebugModulesInitialSnapshot,
	}

	stream := streamui.New(req, ssClient, callOpts)

	if err := pbsubstreams.ValidateRequest(req, false); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("validate request: %w", err)
	}

	toPrint := ctx.DebugModulesOutput
	if toPrint == nil {
		toPrint = []string{ctx.OutputModule}
	}

	replayLogFilePath := filepath.Join(ctx.HomeDir, "replay.log")
	replayLog := replaylog.New(replaylog.WithPath(replayLogFilePath))
	if ctx.Vcr {
		stream.ReplayBundle, err = replayLog.ReadReplay()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, err
		}
	} else {
		if err := replayLog.OpenForWriting(); err != nil {
			return nil, nil, nil, nil, nil, nil, nil, err
		}
		//defer replayLog.Close()
	}

	debugLogPath := filepath.Join(ctx.HomeDir, "debug.log")
	tea.LogToFile(debugLogPath, "gui:")
	fmt.Println("Logging to", debugLogPath)

	msgDescs, err := manifest.BuildMessageDescriptors(pkg)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("building message descriptors: %w", err)
	}

	requestSummary := &request.Summary{
		Manifest:        ctx.ManifestPath,
		Endpoint:        ctx.SubstreamsClientConfig.Endpoint(),
		ProductionMode:  ctx.ProdMode,
		InitialSnapshot: req.DebugInitialStoreSnapshotForModules,
		Docs:            pkg.PackageMeta,
	}

	return stream, msgDescs, replayLog, requestSummary, pkg.Modules, ctx, graph, nil
}

func GetGraph(manifestPath string) (*manifest.ModuleGraph, *pbsubstreams.Package, error) {
	manifestReader := manifest.NewReader(manifestPath)
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
