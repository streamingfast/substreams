package tui2

import (
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/modsearch"
	"github.com/streamingfast/substreams/tui2/components/search"
	"github.com/streamingfast/substreams/tui2/footer"
	"github.com/streamingfast/substreams/tui2/pages/output"
	"github.com/streamingfast/substreams/tui2/pages/progress"
	"github.com/streamingfast/substreams/tui2/pages/request"
	"github.com/streamingfast/substreams/tui2/replaylog"
	"github.com/streamingfast/substreams/tui2/stream"
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
	memoized string
	lastView time.Time

	msgDescs      map[string]*manifest.ModuleDescriptor
	stream        *streamui.Stream
	replayLog     *replaylog.File
	requestConfig *request.Config // all boilerplate to pass down to refresh

	common.Common
	currentModalFunc common.ModalUpdateFunc
	pages            []common.Component
	activePage       page
	footer           *footer.Footer
	showFooter       bool
	error            error
	tabs             *tabs.Tabs
}

func New(reqConfig *request.Config) (*UI, error) {
	c := common.Common{
		Styles: styles.DefaultStyles(),
	}

	out, err := output.New(c, reqConfig.ManifestPath, reqConfig.OutputModule, reqConfig)
	if err != nil {
		return nil, err
	}
	ui := &UI{
		Common: c,
		pages: []common.Component{
			request.New(c),
			progress.New(c),
			out,
		},
		activePage:    progressPage,
		tabs:          tabs.New(c, []string{"Request", "Progress", "Output"}),
		requestConfig: reqConfig,
		replayLog:     replaylog.New(),
	}
	ui.footer = footer.New(c, ui.pages[0])

	return ui, nil
}

func (ui *UI) Init() tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, ui.restartStream())
	for _, pg := range ui.pages {
		cmds = append(cmds, pg.Init())
	}

	cmds = append(cmds,
		ui.tabs.Init(),
		ui.footer.Init(),
	)

	cmds = append(cmds, tabs.SelectTabCmd(1))

	return tea.Batch(cmds...)
}

func (ui *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if bundle, ok := msg.(streamui.ReplayBundle); ok {
		var seq []tea.Cmd
		for _, el := range bundle {
			el := el
			seq = append(seq, func() tea.Msg { return el })
		}
		return ui, tea.Sequence(seq...)
	}
	if ui.replayLog != nil {
		if err := ui.replayLog.Push(msg); err != nil {
			log.Printf("Failed to push to replay log: %s", err.Error())
			return ui, tea.Quit
		}
	}
	return ui.update(msg)
}
func (ui *UI) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		ui.forceRefresh()
		ui.SetSize(msg.Width, msg.Height)
	case common.SetModalUpdateFuncMsg:
		ui.currentModalFunc = common.ModalUpdateFunc(msg)
	case search.ApplySearchQueryMsg:
		ui.currentModalFunc = nil
	case modsearch.ApplyModuleSearchQueryMsg:
		ui.currentModalFunc = nil
	case tea.KeyMsg:
		ui.forceRefresh()
		if msg.String() == "ctrl+c" {
			return ui, tea.Quit
		}
		if ui.currentModalFunc != nil {
			_, cmd := ui.currentModalFunc(msg)
			cmds = append(cmds, cmd)
			return ui, tea.Batch(cmds...)
		}
		switch msg.String() {
		case "q":
			return ui, tea.Quit
		case "?":
			ui.footer.SetShowAll(!ui.footer.ShowAll())
			ui.SetSize(ui.Width, ui.Height)
		case "r":
			return ui, ui.restartStream()
		}
		_, cmd := ui.tabs.Update(msg)
		cmds = append(cmds, cmd)
		_, cmd = ui.pages[ui.activePage].Update(msg)
		cmds = append(cmds, cmd)
		return ui, tea.Batch(cmds...)
	case request.NewRequestInstance:
		ui.stream = msg.Stream
		ui.msgDescs = msg.MsgDescs
		ui.replayLog = msg.ReplayLog
		ui.requestConfig = msg.RefreshCtx
		cmds = append(cmds, ui.stream.Init())
	case streamui.Msg:
		switch msg {
		case streamui.ConnectingMsg:
		case streamui.EndOfStreamMsg:
		case streamui.InterruptStreamMsg:
		}
	case tabs.SelectTabMsg:
		ui.forceRefresh()
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
		ui.SetSize(ui.Width, ui.Height)
	case tabs.ActiveTabMsg:
		ui.forceRefresh()
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
		ui.SetSize(ui.Width, ui.Height) // For when the footer changes size here
	}

	if ui.stream != nil {
		cmds = append(cmds, ui.stream.Update(msg))
	}

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

func (ui *UI) forceRefresh() {
	ui.lastView = time.Time{}
}

func (ui *UI) View() string {
	headline := ui.Styles.Header.Render("Substreams GUI")

	if ui.stream != nil {
		var color lipgloss.TerminalColor
		switch ui.stream.StreamStatus() {
		case stream.StatusRunning:
			color = ui.Styles.StreamRunningColor
		case stream.StatusStopped:
			color = ui.Styles.StreamStoppedColor
		case stream.StatusError:
			color = ui.Styles.StreamErrorColor
		}
		headline = ui.Styles.Header.Copy().Foreground(color).Render("Substreams GUI")
	}

	return lipgloss.JoinVertical(0,
		headline,
		ui.Styles.Tabs.Render(ui.tabs.View()),
		ui.pages[ui.activePage].View(),
		ui.footer.View(),
	)
}

func (ui *UI) restartStream() tea.Cmd {
	ui.stream = nil
	requestInstance, err := ui.requestConfig.NewInstance()
	if err != nil {
		return func() tea.Msg {
			return streamui.StreamErrorMsg(err)
		}
	}

	return tea.Sequence(
		func() tea.Msg {
			return streamui.InterruptStreamMsg
		},
		func() tea.Msg {
			return request.NewRequestInstance(requestInstance)
		},

		func() tea.Msg {
			if ui.replayLog.IsWriting() {
				return ui.replayLog
			}
			return nil
		},
	)
}
