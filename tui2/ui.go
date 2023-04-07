package tui2

import (
	"log"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/replaylog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jhump/protoreflect/desc"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/footer"
	"github.com/streamingfast/substreams/tui2/pages/output"
	"github.com/streamingfast/substreams/tui2/pages/progress"
	"github.com/streamingfast/substreams/tui2/pages/request"
	"github.com/streamingfast/substreams/tui2/stream"
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
	msgDescs  map[string]*desc.MessageDescriptor
	stream    *stream.Stream
	replayLog *replaylog.File

	common.Common
	pages      []common.Component
	activePage page
	footer     *footer.Footer
	showFooter bool
	error      error
	tabs       *tabs.Tabs
}

func New(stream *stream.Stream, msgDescs map[string]*desc.MessageDescriptor, vcr *replaylog.File, reqSummary *request.Summary, modules *pbsubstreams.Modules) *UI {
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
			output.New(c, msgDescs),
		},
		activePage: progressPage,
		tabs:       tabs.New(c, []string{"Request", "Progress", "Output"}),
		replayLog:  vcr,
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
	if bundle, ok := msg.(stream.ReplayBundle); ok {
		for _, el := range bundle {
			_, _ = ui.update(el)
		}
	}
	if err := ui.replayLog.Push(msg); err != nil {
		log.Printf("Failed to push to vcr: %s", err.Error())
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
		}
		_, cmd := ui.tabs.Update(msg)
		cmds = append(cmds, cmd)
		_, cmd = ui.pages[ui.activePage].Update(msg)
		cmds = append(cmds, cmd)
		return ui, tea.Batch(cmds...)

	case stream.Msg:
		switch msg {
		case stream.ConnectingMsg:
		case stream.EndOfStreamMsg:
		case stream.InterruptStreamMsg:

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
