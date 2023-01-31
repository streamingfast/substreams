package tui2

import (
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
	msgDescs map[string]*desc.MessageDescriptor
	stream   *stream.Stream

	common     common.Common
	pages      []common.Component
	activePage page
	footer     *footer.Footer
	showFooter bool
	error      error
	tabs       *tabs.Tabs
}

func New(stream *stream.Stream, msgDescs map[string]*desc.MessageDescriptor) *UI {
	c := common.Common{
		Styles: styles.DefaultStyles(),
	}
	ui := &UI{
		msgDescs: msgDescs,
		stream:   stream,
		pages: []common.Component{
			request.New(c),
			progress.New(c),
			output.New(c),
		},
		tabs: tabs.New(c, []string{"Request", "Progress", "Output"}),
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

	return tea.Batch(cmds...)
}

func (ui *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return ui, tea.Quit
		}
	case stream.Msg:
		switch msg {
		case stream.ConnectingMsg:
		case stream.EndOfStreamMsg:
		case stream.InterruptStreamMsg:

		}
	case stream.SetRequestMsg:

	//case stream.SetMessageDescriptors:
	//	ui.msgDescs = msg
	case tabs.ActiveTabMsg:
		ui.activePage = page(msg)
		ui.footer.SetKeyMap(ui.pages[ui.activePage])
	}

	_, cmd := ui.footer.Update(msg)
	cmds = append(cmds, cmd)
	_, cmd = ui.tabs.Update(msg)
	cmds = append(cmds, cmd)
	_, cmd = ui.pages[ui.activePage].Update(msg)
	cmds = append(cmds, cmd)

	return ui, tea.Batch(cmds...)
}

func (ui *UI) View() string {
	return lipgloss.JoinVertical(0,
		"Substreams GUI",
		ui.tabs.View(),
		ui.pages[ui.activePage].View(),
		ui.footer.View(),
	)
}
