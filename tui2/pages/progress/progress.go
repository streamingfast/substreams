package progress

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/dustin/go-humanize"

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

	progressView    viewport.Model
	progressUpdates int
	dataPayloads    int
	slowestJobs     []string
	slowestModules  []string

	totalBytesRead    uint64
	totalBytesWritten uint64

	initCheckpointBlockCount uint64
	lastCheckpointTime       time.Time
	lastCheckpointBlocks     uint64
	lastCheckpointBlockRate  uint64

	maxParallelWorkers uint64
	bars               *ranges.Bars

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
		msg := msg.(*pbsubstreamsrpc.ModulesProgress)
		newBars := make([]*ranges.Bar, len(msg.Stages))
		newStageModules := make([]string, len(msg.Stages))

		var totalProcessedBlocks uint64

		sort.Slice(msg.RunningJobs, func(i, j int) bool {
			return msg.RunningJobs[i].DurationMs > msg.RunningJobs[j].DurationMs
		})
		newSlowestJobs := make([]string, 4)

		jobsPerStage := make([]int, len(msg.Stages))
		for i, j := range msg.RunningJobs {
			totalProcessedBlocks += j.ProcessedBlocks
			jobsPerStage[j.Stage]++
			if i < len(newSlowestJobs) {
				newSlowestJobs[i] = fmt.Sprintf("[Stage: %d, Range: %d-%d, Duration: %ds]", j.Stage, j.StartBlock, j.StopBlock, j.DurationMs/1000)
			}
		}

		for i, stage := range msg.Stages {
			newStageModules[i] = strings.Join(stage.Modules, ",")

			jobsForStage := jobsPerStage[i]
			displayedName := fmt.Sprintf("stage %d (%d jobs)", i, jobsForStage)

			br := make([]*ranges.BlockRange, len(stage.CompletedRanges))
			for j, r := range stage.CompletedRanges {

				totalProcessedBlocks += (r.EndBlock - r.StartBlock)
				br[j] = &ranges.BlockRange{
					Start: r.StartBlock,
					End:   r.EndBlock,
				}
			}

			newBar := p.bars.NewBar(displayedName, br, stage.Modules)
			newBars[i] = newBar
		}

		var newSlowestModules []string
		sort.Slice(msg.ModulesStats, func(i, j int) bool {
			return msg.ModulesStats[i].TotalProcessingTimeMs/(msg.ModulesStats[i].TotalProcessedBlockCount+1) > msg.ModulesStats[j].TotalProcessingTimeMs/(msg.ModulesStats[j].TotalProcessedBlockCount+1)
		})
		var moduleNameLen int
		for _, mod := range msg.ModulesStats {
			if len(mod.Name) > moduleNameLen {
				moduleNameLen = len(mod.Name)
			}
		}
		for i, mod := range msg.ModulesStats {
			totalBlocks := mod.TotalProcessedBlockCount + 1

			ratio := mod.TotalProcessingTimeMs / totalBlocks
			if i > 3 {
				break
			}
			var externalMetrics string
			for _, ext := range mod.ExternalCallMetrics {
				externalMetrics += fmt.Sprintf(" [%s (%d): %d%%]", ext.Name, ext.Count, ext.TimeMs/mod.TotalProcessingTimeMs)
			}
			var storeMetrics string
			if mod.TotalStoreOperationTimeMs != 0 {
				storeMetrics = fmt.Sprintf(" [store (%d read/blk, %d write/blk, %d deletePrefix/blk): %d%%]",
					mod.TotalStoreReadCount/totalBlocks,
					mod.TotalStoreWriteCount/totalBlocks,
					mod.TotalStoreDeleteprefixCount/totalBlocks,
					mod.TotalStoreOperationTimeMs/mod.TotalProcessingTimeMs)
			}
			newSlowestModules = append(newSlowestModules, fmt.Sprintf("%*s %8sms per block%s%s", moduleNameLen, mod.Name, humanize.Comma(int64(ratio)), storeMetrics, externalMetrics))
		}

		var mustResize bool
		p.slowestJobs = newSlowestJobs
		if len(newSlowestModules) != len(p.slowestModules) {
			mustResize = true
		}
		p.slowestModules = newSlowestModules

		if elapsed := time.Since(p.lastCheckpointTime); elapsed > 900*time.Millisecond {
			if p.lastCheckpointBlocks == 0 {
				p.initCheckpointBlockCount = totalProcessedBlocks
			} else {
				blockDiff := totalProcessedBlocks - p.lastCheckpointBlocks
				p.lastCheckpointBlockRate = blockDiff * 1000 / uint64(elapsed.Milliseconds())
			}
			p.lastCheckpointBlocks = totalProcessedBlocks
			p.lastCheckpointTime = time.Now()
		}

		if msg.ProcessedBytes != nil {
			p.totalBytesRead = msg.ProcessedBytes.TotalBytesRead
			p.totalBytesWritten = msg.ProcessedBytes.TotalBytesWritten
		}

		if mustResize {
			p.SetSize(p.Common.Width, p.Common.Height)
		}
		p.bars.Update(newBars)
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
	"Bytes Read / Written: ",
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
	maxWorkers := ""
	if p.maxParallelWorkers != 0 {
		maxWorkers = fmt.Sprintf(", %d max workers", p.maxParallelWorkers)
	}
	infos := []string{
		fmt.Sprintf("%d (%d per second%s)", p.lastCheckpointBlocks-p.initCheckpointBlockCount, p.lastCheckpointBlockRate, maxWorkers),
		fmt.Sprintf("%s / %s", humanize.Bytes(p.totalBytesRead), humanize.Bytes(p.totalBytesWritten)),
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

	slowestJobs := "Slowest jobs:\n"
	for _, job := range p.slowestJobs {
		if job == "" {
			slowestJobs += "\n"
		} else {
			slowestJobs += " - " + job + "\n"
		}
	}

	slowestModules := "Slowest modules:\n"
	for _, mod := range p.slowestModules {
		if mod == "" {
			slowestModules += "\n"
		} else {
			slowestModules += " - " + mod + "\n"
		}
	}

	return lipgloss.JoinVertical(0,
		lipgloss.NewStyle().Margin(0, 2).Render(lipgloss.JoinHorizontal(0,
			lipgloss.JoinVertical(1, labels...),
			lipgloss.JoinVertical(0, infos...),
		)),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(p.progressView.View()),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(lipgloss.NewStyle().MarginLeft(10).Render(slowestJobs)),
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(lipgloss.NewStyle().MarginLeft(10).Render(slowestModules)),
	)

}

func (p *Progress) SetSize(w, h int) {
	headerHeight := 7
	footerHeight := 6 + len(p.slowestModules) + len(p.slowestJobs)
	p.Common.SetSize(w, h)
	if p.bars != nil {
		p.bars.SetSize(w-2 /* borders */, h-headerHeight)
	}
	p.progressView.Width = w
	p.progressView.Height = h - headerHeight - footerHeight
	p.Styles.StatusBarValue.Width(p.Common.Width - labelsMaxLen()) // adjust status bar width to force word wrap: full width - labels width
}
