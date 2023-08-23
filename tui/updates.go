package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// Implement the tea.Model interface
func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg {
	case Connecting:
		m.Connected = false
	case Connected:
		m.Connected = true
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width < 45 {
			m.BarSize = 4
		} else {
			m.BarSize = uint64(msg.Width) - 45
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			m.ui.Cancel()
			fmt.Println("Interrupted UI")
			return m, tea.Quit
		}
		switch msg.String() {
		case "m":
			m.BarMode = !m.BarMode
			return m, nil
		case "q":
			return m, tea.Quit
		}
	case *pbsubstreamsrpc.Request:
		m.Request = msg
		// It's ok to use `StartBlockNum` directly instead of effective start block (start block
		// of cursor if present, `StartBlockNum` otherwise) because this is only used in backprocessing
		// `barmode` which is effective only when no cursor has been passed yet.
		if m.Request.StartBlockNum > 0 {
			m.BackprocessingCompleteAtBlock = uint64(m.Request.StartBlockNum)
		}
		return m, nil
	case *pbsubstreamsrpc.Response_Session:
		m.TraceID = msg.Session.TraceId
		m.BackprocessingCompleteAtBlock = msg.Session.ResolvedStartBlock

	case *pbsubstreamsrpc.ModulesProgress:
		m.Updates += 1
		thisSec := time.Now().Unix()
		if m.UpdatedSecond != thisSec {
			m.UpdatesPerSecond = m.UpdatesThisSecond
			m.UpdatesThisSecond = 0
			m.UpdatedSecond = thisSec
		}
		m.UpdatesThisSecond += 1

		newStageProgress := updatedRanges{}
		newStageModules := make([]string, len(msg.Stages))

		sort.Slice(msg.RunningJobs, func(i, j int) bool {
			return msg.RunningJobs[i].DurationMs > msg.RunningJobs[j].DurationMs
		})
		var newSlowestJobs []string

		jobsPerStage := make([]int, len(msg.Stages))
		for _, j := range msg.RunningJobs {
			jobsPerStage[j.Stage]++
			if j.DurationMs > 5000 && len(newSlowestJobs) < 5 {
				newSlowestJobs = append(newSlowestJobs, fmt.Sprintf("[Stage: %d, Range: %d-%d, Duration: %ds]", j.Stage, j.StartBlock, j.StopBlock, j.DurationMs/1000))
			}
		}

		for i, stage := range msg.Stages {
			newStageModules[i] = strings.Join(stage.Modules, ",")

			jobsForStage := jobsPerStage[i]
			displayedName := fmt.Sprintf("stage %d (%d jobs)", i, jobsForStage)

			ranges := make([]*blockRange, len(stage.CompletedRanges))
			for j, r := range stage.CompletedRanges {
				ranges[j] = &blockRange{
					Start: r.StartBlock,
					End:   r.EndBlock,
				}
			}
			newStageProgress[displayedName] = ranges
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
			if ratio < 10 || i > 4 {
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
			newSlowestModules = append(newSlowestModules, fmt.Sprintf("%*s - %8sms per block%s%s", moduleNameLen, mod.Name, humanize.Comma(int64(ratio)), storeMetrics, externalMetrics))
		}

		m.SlowModules = newSlowestModules
		m.StagesProgress = newStageProgress
		m.StagesModules = newStageModules
		m.SlowJobs = newSlowestJobs
	default:
	}

	return m, nil
}
