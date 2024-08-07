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
	"github.com/streamingfast/substreams/tui2/styles"
)

type Progress struct {
	common.Common

	started     time.Time
	state       string
	replayState string
	targetBlock uint64

	progressView     viewport.Model
	dataPayloads     int
	slowestJobs      []string
	slowestModules   []string
	slowestSquashing []string

	bars *ranges.Bars
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
	var outCmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "m":
			p.bars.Mode = (p.bars.Mode + 1) % 3
			p.progressView.SetContent(p.bars.View())
		}
		p.progressView, outCmd = p.progressView.Update(msg)

	case *pbsubstreamsrpc.SessionInit:
		linearHandoff := msg.LinearHandoffBlock
		p.targetBlock = msg.ResolvedStartBlock
		p.dataPayloads = 0
		p.bars = ranges.NewBars(p.Common, linearHandoff)
		p.bars.Init()
	case *pbsubstreamsrpc.BlockScopedData:
		p.dataPayloads += 1
	case *pbsubstreamsrpc.ModulesProgress:
		newBars := make([]*ranges.Bar, len(msg.Stages))

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
					displayedModules[i] += lipgloss.NewStyle().Foreground(styles.StreamErrorColor).Render("(S)")
				}
			}

			jobsForStage := jobsPerStage[i]
			displayedName := fmt.Sprintf("stage %d (%d jobs)", i, jobsForStage)

			br := make([]*ranges.BlockRange, len(stage.CompletedRanges))
			for j, r := range stage.CompletedRanges {
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

		if mustResize {
			p.SetSize(p.Common.Width, p.Common.Height)
		}
		p.bars.Update(newBars)
		p.progressView.SetContent(p.bars.View())
	case *replaylog.File:
		p.replayState = " [saving to replay log]"
	}

	return p, outCmd
}

var labels = []string{
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
		fmt.Sprintf("%d", p.targetBlock),
		fmt.Sprintf("%d", p.dataPayloads),
		styles.StatusBarValue.Render(p.state + p.replayState),
	}

	components := []string{
		lipgloss.NewStyle().Margin(0, 2).Render(lipgloss.JoinHorizontal(0,
			lipgloss.JoinVertical(1, labels...),
			lipgloss.JoinVertical(0, infos...),
		)),
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
		components = append(components, lipgloss.NewStyle().MarginLeft(2).Foreground(styles.StreamErrorColor).Render("Slow Squashing: "+strings.Join(p.slowestSquashing, ", ")))
	}

	return lipgloss.JoinVertical(0, components...)
}

func (p *Progress) SetSize(w, h int) {
	headerHeight := 8
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
	styles.StatusBarValue.Width(p.Common.Width - labelsMaxLen()) // adjust status bar width to force word wrap: full width - labels width
}
