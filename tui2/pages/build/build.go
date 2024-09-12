package build

import (
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/tui2/buildoutput"
	"github.com/streamingfast/substreams/tui2/common"
)

type Build struct {
	common.Common

	ManifestPath       string
	BuildOutputMsgs    []string
	BuildOutputErrMsgs []string
	buildView          viewport.Model
	params             map[string][]string
}

func New(c common.Common, manifestPath string) *Build {
	page := &Build{
		Common:          c,
		ManifestPath:    manifestPath,
		buildView:       viewport.New(c.Width, c.Height),
		params:          make(map[string][]string),
		BuildOutputMsgs: []string{},
	}
	return page
}

func (b *Build) Init() tea.Cmd {
	return b.buildView.Init()
}

func (d *Build) SetSize(w, h int) {
	d.Common.SetSize(w, h)
	d.buildView.Height = max(h-2 /* for borders */, 0)
	d.buildView.Width = w
}

func (b *Build) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case string:
		log.Println(msg)
	case NewBuildInstance:
		b.BuildOutputMsgs = []string{}
		b.BuildOutputErrMsgs = []string{}
	case buildoutput.BuildOutputMsg:
		b.BuildOutputMsgs = append(b.BuildOutputMsgs, msg.Msg)
	}
	var cmd tea.Cmd
	b.buildView, cmd = b.buildView.Update(msg)
	return b, cmd
}

func (b *Build) View() string {
	outputMsgs := strings.Join(b.BuildOutputMsgs, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(b.Width - 2).
		Height(b.buildView.Height + 1 /* for borders */).
		Render(
			outputMsgs,
		)
}
