package tui2

import (
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/errorbox"
	"github.com/streamingfast/substreams/tui2/footer"
	"github.com/streamingfast/substreams/tui2/pages/build"
	"github.com/streamingfast/substreams/tui2/pages/docs"
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
	docsPage
	buildPage
)

type UI struct {
	lastView time.Time

	msgDescs      map[string]*manifest.ModuleDescriptor
	stream        *streamui.Stream
	replayLog     *replaylog.File
	requestConfig *request.Config // all boilerplate to pass down to refresh

	common.Common
	modalComponent common.Component
	pages          []common.Component
	activePage     page
	footer         *footer.Footer
	tabs           *tabs.Tabs
}

func New(reqConfig *request.Config) (*UI, error) {
	c := common.Common{}

	outputTab, err := output.New(c, reqConfig)
	if err != nil {
		return nil, err
	}
	ui := &UI{
		Common: c,
		pages: []common.Component{
			request.New(c, reqConfig),
			progress.New(c),
			outputTab,
			docs.New(c),
			build.New(c),
		},
		activePage:    requestPage,
		tabs:          tabs.New(c, []string{"Request", "Backprocessing", "Output", "Docs", "Build"}),
		requestConfig: reqConfig,
		replayLog:     replaylog.New(),
	}
	ui.footer = footer.New(c, ui.pages[ui.activePage])

	return ui, nil
}

func (ui *UI) Init() tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, ui.setupNewInstance(false))
	for _, pg := range ui.pages {
		cmds = append(cmds, pg.Init())
	}

	cmds = append(cmds,
		ui.tabs.Init(),
		ui.footer.Init(),
	)

	cmds = append(cmds, tabs.SelectTabCmd(int(requestPage)))

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
	case stream.StreamErrorMsg:
		cmds = append(cmds, common.SetModalComponentCmd(errorbox.New(ui.Common, msg.Error())))
	case common.SetModalComponentMsg:
		log.Printf("Setting modal component %T", msg)
		if msg != nil {
			cmds = append(cmds, msg.Init())
		}
		ui.modalComponent = msg
		ui.resize()
	case common.CancelModalMsg:
		ui.modalComponent = nil
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
	case request.SetupNewInstanceMsg:
		return ui, ui.setupNewInstance(msg.StartStream)
	case tea.KeyMsg:
		ui.forceRefresh()
		if msg.String() == "ctrl+c" {
			return ui, tea.Quit
		}
		if ui.modalComponent != nil {
			break
		}
		switch msg.String() {
		case "q":
			return ui, tea.Quit
		case "?":
			ui.footer.SetShowAll(!ui.footer.ShowAll())
			ui.resize()
		case "r":
			return ui, request.SetupNewInstanceCmd(true)
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
		if msg.StartStream {
			cmds = append(cmds, ui.stream.Init())
		}
	case streamui.Msg:
		switch msg {
		case streamui.BackprocessingMsg:
			cmds = append(cmds, tabs.SelectTabCmd(int(progressPage)))
		case streamui.StreamingMsg:
			cmds = append(cmds, tabs.SelectTabCmd(int(outputPage)))
		}
	case tabs.SelectTabMsg:
		ui.forceRefresh()
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
		ui.resize()
	case tabs.ActiveTabMsg:
		ui.forceRefresh()
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
		ui.resize()
	}

	if ui.stream != nil {
		cmds = append(cmds, ui.stream.Update(msg))
	}

	var holdModalKeys bool
	if ui.modalComponent != nil {
		m, cmd := ui.modalComponent.Update(msg)
		ui.modalComponent = m.(common.Component)
		cmds = append(cmds, cmd)
		holdModalKeys = true
	}

	if _, isKeyMsg := msg.(tea.KeyMsg); !holdModalKeys || !isKeyMsg {
		_, cmd := ui.footer.Update(msg)
		cmds = append(cmds, cmd)
		_, cmd = ui.tabs.Update(msg)
		cmds = append(cmds, cmd)
		for _, pg := range ui.pages {
			if _, cmd = pg.Update(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return ui, tea.Batch(cmds...)
}

func (ui *UI) SetSize(w, h int) {
	ui.Common.SetSize(w, h)
	ui.resize()
}
func (ui *UI) resize() {
	footerHeight := ui.footer.Height()
	ui.footer.SetSize(ui.Width, footerHeight)
	ui.tabs.SetSize(ui.Width, 3)
	if ui.modalComponent != nil {
		// -2 for border at render time.
		ui.modalComponent.SetSize(ui.Width-2, ui.Height-footerHeight-2)
	}
	for _, pg := range ui.pages {
		pg.SetSize(ui.Width, ui.Height-ui.tabs.Height-footerHeight)
	}
}

func (ui *UI) forceRefresh() {
	ui.lastView = time.Time{}
}

func (ui *UI) View() string {
	ui.tabs.LogoStyle = styles.Logo

	if ui.stream != nil {
		var color lipgloss.TerminalColor
		switch ui.stream.StreamStatus() {
		case streamui.StatusRunning:
			color = styles.StreamRunningColor
		case streamui.StatusStopped:
			color = styles.StreamStoppedColor
		case streamui.StatusError:
			color = styles.StreamErrorColor
		}
		ui.tabs.LogoStyle = styles.Logo.Foreground(color)
	}

	main := lipgloss.JoinVertical(0,
		styles.Tabs.Render(ui.tabs.View()),
		ui.footer.View(),
		ui.pages[ui.activePage].View(),
	)

	if ui.modalComponent != nil {
		_, ok := ui.modalComponent.(common.IsInlineModal)
		if !ok {
			_, fullWidth := ui.modalComponent.(common.IsFullWidthModal)
			contents := ui.modalComponent.View()
			style := styles.ModalBox
			if fullWidth {
				style = styles.FullWidthModalBox.Width(ui.Width - 2)
			}
			modalView := style.Render(contents)
			x := ui.Width/2 - lipgloss.Width(modalView)/2
			y := ui.Height/2 - lipgloss.Height(modalView)/2
			if fullWidth {
				x = 0
			}
			main = styles.PlaceOverlay(x, y, modalView, main, true)
		}
	}

	return main
}

func (ui *UI) setupNewInstance(startStream bool) tea.Cmd {
	var cmds []tea.Cmd
	ui.stream = nil
	reqInstance, err := ui.requestConfig.NewInstance()
	if err != nil {
		return func() tea.Msg { return streamui.StreamErrorMsg(err) }
	}
	reqInstance.StartStream = startStream

	if startStream {
		cmds = append(cmds,
			func() tea.Msg { return streamui.InterruptStreamMsg },
		)
	}
	cmds = append(cmds, func() tea.Msg {
		return request.NewRequestInstance(reqInstance)
	})
	if startStream {
		cmds = append(cmds,
			func() tea.Msg {
				if ui.replayLog.IsWriting() {
					return ui.replayLog
				}
				return nil
			},
		)

	}

	return tea.Sequence(cmds...)
}
