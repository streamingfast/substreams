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

	started     time.Time
	state       string
	replayState string
	targetBlock uint64

	progressView     viewport.Model
	progressUpdates  int
	dataPayloads     int
	slowestJobs      []string
	slowestModules   []string
	slowestSquashing []string

	totalBytesRead    uint64
	totalBytesWritten uint64

	initCheckpointBlockCount uint64
	lastCheckpointTime       time.Time
	lastCheckpointBlocks     uint64
	lastCheckpointBlockRate  uint64

	maxParallelWorkers uint64
	bars               *ranges.Bars

	curErr          string
	curErrFormated  string
	curErrLineCount int
}

func New(c common.Common) *Progress {
	return &Progress{
		Common:       c,
		state:        "Initializing",
		targetBlock:  0,
		progressView: viewport.New(24, 80),
		bars:         ranges.NewBars(c, 0),
		started:      time.Now(),
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

		var totalProcessedBlocks uint64

		sort.Slice(msg.RunningJobs, func(i, j int) bool {
			if msg.RunningJobs[i].DurationMs == 0 {
				return false
			}
			if msg.RunningJobs[j].DurationMs == 0 {
				return true
			}

			return msg.RunningJobs[i].ProcessedBlocks*100000/msg.RunningJobs[i].DurationMs < msg.RunningJobs[j].ProcessedBlocks*100000/msg.RunningJobs[j].DurationMs
		})
		var newSlowestJobs []string

		incompleteRanges := make(map[int][]*ranges.BlockRange)
		jobsPerStage := make([]int, len(msg.Stages))
		slowJobCount := 0
		for _, j := range msg.RunningJobs {
			totalProcessedBlocks += j.ProcessedBlocks
			jobsPerStage[j.Stage]++
			if slowJobCount < 4 && j.DurationMs > 10000 { // skip 'young' jobs
				newSlowestJobs = append(newSlowestJobs, fmt.Sprintf("[Stage: %d, Range: %d-%d, Duration: %ds, Blocks/sec: %.1f]", j.Stage, j.StartBlock, j.StopBlock, j.DurationMs/1000, float64(j.ProcessedBlocks)/float64(j.DurationMs/1000)))
				slowJobCount++
			}

			incompleteRanges[int(j.Stage)] = append(incompleteRanges[int(j.Stage)], &ranges.BlockRange{Start: j.StartBlock, End: j.StartBlock + j.ProcessedBlocks})
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
		var newSlowestSquashing []string
		squashingModules := make(map[string]bool)
		for i, mod := range msg.ModulesStats {
			totalBlocks := mod.TotalProcessedBlockCount + 1
			if mod.StoreCurrentlyMerging {
				squashingModules[mod.Name] = true
				if percent := mod.TotalStoreMergingTimeMs * 100 / uint64(time.Since(p.started).Milliseconds()); percent > 15 {
					newSlowestSquashing = append(newSlowestSquashing, fmt.Sprintf("%s (%d%%)", mod.Name, percent))
				}
			}

			ratio := mod.TotalProcessingTimeMs / totalBlocks
			if i > 3 || ratio < 50 {
				continue
			}
			var externalMetrics string
			for _, ext := range mod.ExternalCallMetrics {
				externalMetrics += fmt.Sprintf(" [%s (%d): %d%%]", ext.Name, ext.Count, ext.TimeMs*100/mod.TotalProcessingTimeMs)
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

		for i, stage := range msg.Stages {
			displayedModules := make([]string, len(stage.Modules))
			for i := range stage.Modules {
				displayedModules[i] = stage.Modules[i]
				if squashingModules[stage.Modules[i]] {
					displayedModules[i] += lipgloss.NewStyle().Foreground(p.Styles.StreamErrorColor).Render("(S)")
				}
			}

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
			br = append(br, incompleteRanges[i]...)

			sort.Slice(br, func(i, j int) bool { return br[i].Start < br[j].Start })
			newBar := p.bars.NewBar(displayedName, br, displayedModules)
			newBars[i] = newBar
		}

		var mustResize bool
		if len(newSlowestJobs) != len(p.slowestJobs) {
			mustResize = true
		}
		p.slowestJobs = newSlowestJobs
		if len(newSlowestModules) != len(p.slowestModules) {
			mustResize = true
		}
		p.slowestModules = newSlowestModules
		if len(newSlowestSquashing) != len(p.slowestSquashing) {
			mustResize = true
		}
		p.slowestSquashing = newSlowestSquashing

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
		p.state = "Error"
		p.curErr = msg.(stream.StreamErrorMsg).Error()
		p.SetSize(p.Common.Width, p.Common.Height)

		return p, nil
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

	components := []string{
		lipgloss.NewStyle().Margin(0, 2).Render(lipgloss.JoinHorizontal(0,
			lipgloss.JoinVertical(1, labels...),
			lipgloss.JoinVertical(0, infos...),
		)),
	}

	if p.state == "Error" {
		components = append(components, lipgloss.NewStyle().Background(p.Styles.StreamErrorColor).Width(p.Width).Render(p.curErrFormated))
	}

	components = append(components,
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(p.progressView.View()),
	)

	if p.slowestJobs != nil {
		slowestJobs := lipgloss.JoinHorizontal(lipgloss.Top, "Slowest Jobs:      ", lipgloss.JoinVertical(lipgloss.Left, p.slowestJobs...))
		components = append(components, lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(lipgloss.NewStyle().MarginLeft(1).Render(slowestJobs)))
	}

	if p.slowestModules != nil {
		slowestModules := lipgloss.JoinHorizontal(lipgloss.Top, "Slowest Modules:   ", lipgloss.JoinVertical(lipgloss.Left, p.slowestModules...))
		components = append(components, lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true).Width(p.Width-5).Render(lipgloss.NewStyle().MarginLeft(1).Render(slowestModules)))
	}

	if p.slowestSquashing != nil {
		components = append(components, lipgloss.NewStyle().MarginLeft(2).Foreground(p.Styles.StreamErrorColor).Render("Slow Squashing: "+strings.Join(p.slowestSquashing, ", ")))
	}

	return lipgloss.JoinVertical(0, components...)
}

func (p *Progress) SetSize(w, h int) {
	if p.curErr != "" {
		p.curErrFormated, p.curErrLineCount = wrapString(p.curErr, p.Width) // wrapping and linecount always recomputed on SetSize
	}

	headerHeight := 8 + p.curErrLineCount
	footerHeight := 0
	if p.slowestModules != nil {
		footerHeight += len(p.slowestModules) + 2
	}
	if p.slowestJobs != nil {
		footerHeight += len(p.slowestJobs) + 2
	}
	if p.slowestSquashing != nil {
		footerHeight += 1
	}
	p.Common.SetSize(w, h)
	if p.bars != nil {
		p.bars.SetSize(w-2 /* borders */, h-headerHeight)
	}
	p.progressView.Width = w
	p.progressView.Height = h - headerHeight - footerHeight
	p.Styles.StatusBarValue.Width(p.Common.Width - labelsMaxLen()) // adjust status bar width to force word wrap: full width - labels width
}
