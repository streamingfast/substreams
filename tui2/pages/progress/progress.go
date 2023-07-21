package progress

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"

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

	progressView       viewport.Model
	progressUpdates    int
	dataPayloads       int
	blocksPerSecond    uint64
	blocksThisSecond   uint64
	updatedSecond      int64
	updatesPerSecond   int
	updatesThisSecond  int
	maxParallelWorkers uint64

	bars   *ranges.Bars
	curErr string
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
		switch msg.(tea.KeyMsg).String() {
		case "m":
			p.bars.Mode = (p.bars.Mode + 1) % 3
			p.progressView.SetContent(p.bars.View())
		}
		var cmd tea.Cmd
		p.progressView, cmd = p.progressView.Update(msg)
		cmds = append(cmds, cmd)

	case *pbsubstreamsrpc.SessionInit:
		sessionInit := msg.(*pbsubstreamsrpc.SessionInit)
		linearHandoff := sessionInit.LinearHandoffBlock
		p.targetBlock = sessionInit.ResolvedStartBlock
		p.dataPayloads = 0
		p.maxParallelWorkers = sessionInit.MaxParallelWorkers
		p.bars = ranges.NewBars(p.Common, linearHandoff)
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
			if p.blocksThisSecond > 0 {
				p.blocksPerSecond = p.bars.TotalBlocks - p.blocksThisSecond
			}
			p.blocksThisSecond = p.bars.TotalBlocks
		}
		p.updatesThisSecond += 1
		p.bars.Update(msg)
		p.progressView.SetContent(p.bars.View())
	case stream.StreamErrorMsg:
		p.state = fmt.Sprintf("Error")
		p.curErr = msg.(stream.StreamErrorMsg).Error()
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
	"Parallel engine blocks processed: ",
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

func wrapString(input string, screenWidth int) (string, int) {
	words := strings.Fields(input)
	var wrappedString strings.Builder
	var lineLength int
	lineCount := 1

	for _, word := range words {
		wordLen := len(word)

		if lineLength+wordLen+1 > screenWidth {
			wrappedString.WriteString("\n")
			lineLength = 0
			lineCount++
		}

		if lineLength > 0 {
			wrappedString.WriteString(" ")
			lineLength++
		}

		wrappedString.WriteString(word)
		lineLength += wordLen
	}

	return wrappedString.String(), lineCount
}

func (p *Progress) View() string {
	blocksPerSecondPerModule := ""
	maxWorkers := ""
	if p.maxParallelWorkers != 0 {
		maxWorkers = fmt.Sprintf(", %d max workers", p.maxParallelWorkers)
	}
	if p.bars.BarCount != 0 && p.blocksPerSecond != 0 {
		blocksPerSecondPerModule = fmt.Sprintf(", %d per module", p.blocksPerSecond/p.bars.BarCount)
	}
	infos := []string{
		fmt.Sprintf("%d (%d per second%s%s)", p.bars.TotalBlocks, p.blocksPerSecond, blocksPerSecondPerModule, maxWorkers),
		fmt.Sprintf("%d", p.targetBlock),
		fmt.Sprintf("%d", p.dataPayloads),
		p.Styles.StatusBarValue.Render(p.state + p.replayState),
	}

	if p.state == "Error" {
		errorStringWrapped, lineCount := wrapString(p.curErr, p.Width)

		return lipgloss.JoinVertical(0,
			lipgloss.NewStyle().Margin(0, 2).Render(lipgloss.JoinHorizontal(0,
				lipgloss.JoinVertical(1, labels...),
				lipgloss.JoinVertical(0, infos...),
			)),
			lipgloss.NewStyle().Background(p.Styles.StreamErrorColor).Width(p.Width).Render(errorStringWrapped),
			lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Height(p.progressView.Height-lineCount).Render(p.progressView.View()),
		)
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
		p.bars.SetSize(w-2 /* borders */, h-headerHeight)
	}
	p.progressView.Width = w
	p.progressView.Height = h - headerHeight
	p.Styles.StatusBarValue.Width(p.Common.Width - labelsMaxLen()) // adjust status bar width to force word wrap: full width - labels width
}
