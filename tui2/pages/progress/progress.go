package progress

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/streamingfast/substreams/tui2/pages/request"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/components/ranges"
	"github.com/streamingfast/substreams/tui2/replaylog"
	"github.com/streamingfast/substreams/tui2/stream"
)

type refreshProgress tea.Msg

type Progress struct {
	common.Common

	state       string
	replayState string
	targetBlock uint64

	progressView      viewport.Model
	progressUpdates   int
	dataPayloads      int
	updatedSecond     int64
	updatesPerSecond  int
	updatesThisSecond int

	bars *ranges.Bars
}

func New(c common.Common) *Progress {
	return &Progress{
		Common:       c,
		state:        "Initializing",
		targetBlock:  0,
		progressView: viewport.New(24, 80),
		bars:         ranges.NewBars(c, 0),
	}
}
func (p *Progress) Init() tea.Cmd {
	return tea.Batch(
		p.bars.Init(),
		p.progressView.Init(),
	)
}

func (p *Progress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.(type) {
	case tea.KeyMsg:
		var cmd tea.Cmd
		p.progressView, cmd = p.progressView.Update(msg)
		cmds = append(cmds, cmd)

	case request.NewRequestInstance:
		targetBlock := msg.(request.NewRequestInstance).Stream.TargetParallelProcessingBlock()
		p.dataPayloads = 0
		p.targetBlock = targetBlock
		p.bars = ranges.NewBars(p.Common, targetBlock)
		p.bars.Init()
	case *pbsubstreamsrpc.BlockScopedData:
		p.dataPayloads += 1
	case *pbsubstreamsrpc.ModulesProgress:
		p.progressUpdates += 1
		thisSec := time.Now().Unix()
		if p.updatedSecond != thisSec {
			p.updatesPerSecond = p.updatesThisSecond
			p.updatesThisSecond = 0
			p.updatedSecond = thisSec
		}
		p.updatesThisSecond += 1
		p.bars.Update(msg)
		p.progressView.SetContent(p.bars.View())
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
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(p.progressView.View()),
	)
}

func (p *Progress) SetSize(w, h int) {
	headerHeight := 7
	p.Common.SetSize(w, h)
	if p.bars != nil {
		p.bars.SetSize(w, h-headerHeight)
	}
	p.progressView.Width = w
	p.progressView.Height = h - headerHeight
	p.Styles.StatusBarValue.Width(p.Common.Width - labelsMaxLen()) // adjust status bar width to force word wrap: full width - labels width
}
