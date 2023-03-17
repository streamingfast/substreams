package progress

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/ranges"
	"github.com/streamingfast/substreams/tui2/replaylog"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Progress struct {
	common.Common
	KeyMap KeyMap

	state       string
	replayState string
	targetBlock uint64

	progressUpdates   int
	dataPayloads      int
	updatedSecond     int64
	updatesPerSecond  int
	updatesThisSecond int

	bars *ranges.Bars
}

func New(c common.Common, targetBlock uint64) *Progress {
	return &Progress{
		Common:      c,
		KeyMap:      DefaultKeyMap(),
		state:       "Initializing",
		targetBlock: targetBlock,
		bars:        ranges.NewBars(c, targetBlock),
	}
}

func (p *Progress) Init() tea.Cmd {
	return p.bars.Init()
}

func (p *Progress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case *pbsubstreams.BlockScopedData:
		p.dataPayloads += 1
	case *pbsubstreams.ModulesProgress:
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
	case *replaylog.File:
		p.replayState = " [saving to replay log]"
	}
	switch msg {
	case stream.ConnectingMsg:
		p.state = "Connecting"
	case stream.ConnectedMsg:
		p.state = "Connected"
	case stream.EndOfStreamMsg:
		p.state = "Stream ended"
	case stream.ReplayedMsg:
		p.state = "Replayed from log"
	}

	return p, nil
}

var labels = []string{
	"Parallel engine progress messages: ",
	"Target block: ",
	"Data payloads received: ",
	"Status: ",
}

func labelsMaxLen() int {
	width := 0
	for _, label := range labels {
		if len(label) > width {
			width = len(label)
		}
	}
	return width
}

func (p *Progress) View() string {

	infos := []string{
		fmt.Sprintf("%d (%d block/sec)", p.progressUpdates, p.updatesPerSecond),
		fmt.Sprintf("%d", p.targetBlock),
		fmt.Sprintf("%d", p.dataPayloads),
		p.Styles.StatusBarValue.Render(p.state + p.replayState),
	}

	return lipgloss.JoinVertical(0,
		lipgloss.NewStyle().Margin(0, 2).Render(lipgloss.JoinHorizontal(0,
			lipgloss.JoinVertical(1, labels...),
			lipgloss.JoinVertical(0, infos...),
		)),
		p.bars.View(),
	)
}

func (p *Progress) SetSize(w, h int) {
	headerHeight := 7
	p.Common.SetSize(w, h)
	p.bars.SetSize(w, h-headerHeight)
	p.Styles.StatusBarValue.Width(p.Common.Width - labelsMaxLen()) // adjust status bar width to force word wrap: full width - labels width
}
