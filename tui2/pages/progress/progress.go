package progress

import (
	"fmt"
	"time"

	"github.com/streamingfast/substreams/tui2/components/ranges"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Progress struct {
	common.Common
	KeyMap KeyMap

	state         string
	linearHandoff uint64

	progressUpdates   int
	dataPayloads      int
	updatedSecond     int64
	updatesPerSecond  int
	updatesThisSecond int

	bars *ranges.Bars
}

func New(c common.Common, linearHandoff uint64) *Progress {
	return &Progress{
		Common:        c,
		KeyMap:        DefaultKeyMap(),
		state:         "Initializing",
		linearHandoff: linearHandoff,
		bars:          ranges.NewBars(c, linearHandoff),
	}
}

func (p *Progress) Init() tea.Cmd {
	return p.bars.Init()
}

func (p *Progress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case stream.ResponseDataMsg:
		p.dataPayloads += 1
	case stream.ResponseProgressMsg:
		p.progressUpdates += 1
		thisSec := time.Now().Unix()
		if p.updatedSecond != thisSec {
			p.updatesPerSecond = p.updatesThisSecond
			p.updatesThisSecond = 0
			p.updatedSecond = thisSec
		}
		p.updatesThisSecond += 1
		p.bars.Update(msg)
	case stream.StreamErrorMsg:
		p.state = fmt.Sprintf("Error: %s", msg)
	}
	switch msg {
	case stream.ConnectingMsg:
		p.state = "Connecting"
	case stream.ConnectedMsg:
		p.state = "Connected"
	case stream.EndOfStreamMsg:
		p.state = "Stream ended"
	}

	return p, nil
}

func (p *Progress) View() string {
	labels := []string{
		"Parallel engine progress messages: ",
		"target block: ",
		"Data payloads received: ",
		"Status: ",
	}
	infos := []string{
		fmt.Sprintf("%d (%d block/sec)", p.progressUpdates, p.updatesPerSecond),
		fmt.Sprintf("%d", p.linearHandoff),
		fmt.Sprintf("%d", p.dataPayloads),
		p.Styles.StatusBarValue.Render(p.state),
	}

	vp := viewport.New(p.Width, p.Height)
	//vp.Style = lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), true)
	vp.SetContent(lipgloss.JoinVertical(0,
		lipgloss.NewStyle().Margin(1, 2).Render(lipgloss.JoinHorizontal(0,
			lipgloss.JoinVertical(1, labels...),
			lipgloss.JoinVertical(0, infos...),
		)),
		p.bars.View(),
	))
	return vp.View()
}

func (p *Progress) SetSize(w, h int) {
	headerHeight := 7
	p.Common.SetSize(w, h)
	p.bars.SetSize(w, h-headerHeight)
}