package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/streamingfast/substreams/internal/formatx"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

const (
	// moduleCostFloorMs is the per-block cost under which a module is not worth a line. A
	// diagnostic that fires on a healthy run is worse than no diagnostic at all.
	moduleCostFloorMs = 10.0
	maxModuleRows     = 12
	maxStageRows      = 4

	// progressLabelWidth is the width of the leftmost column across every row of the block, so
	// that all bars start at the same screen column. "Backprocessing" is the longest label and
	// therefore sets it.
	progressLabelWidth = len("Backprocessing")
)

// progressRow lays out one "<label> <bar> <percent>  <details>" line. Every bar in the block
// goes through it, which is what keeps them aligned.
func progressRow(label string, ratio float64, width int, details ...string) string {
	row := fmt.Sprintf("%s  %s %3.0f%%", padLabel(label), renderBar(ratio, width), ratio*100)

	if detail := strings.Join(details, "  "); detail != "" {
		row += "  " + detail
	}

	return row
}

// stageLabel indents a stage under the header while keeping its bar on the same column.
func stageLabel(name string) string {
	return "  " + name
}

// padLabel pads to progressLabelWidth counting runes, not bytes: the elision label is a
// single-column "…" that fmt's %-Ns would pad as though it were three characters wide.
func padLabel(label string) string {
	if pad := progressLabelWidth - utf8.RuneCountInString(label); pad > 0 {
		return label + strings.Repeat(" ", pad)
	}

	return label
}

// globalDone is the amount of work completed, in module-blocks.
//
// ProcessedBlocks alone is a staircase: it only advances when a job completes. Adding the
// progress of the jobs currently in flight makes it advance continuously, which is what
// makes a rate derived from it stable.
func globalDone(progress *pbsubstreamsrpc.ModulesProgress) uint64 {
	if progress == nil {
		return 0
	}

	done := progress.ProcessedBlocks
	for _, job := range progress.RunningJobs {
		done += job.ProgressBlocks
	}
	return done
}

// globalTotal is the work left to do at the time the session started, in module-blocks,
// excluding whatever was already cached.
func globalTotal(session *pbsubstreamsrpc.SessionInit) uint64 {
	if session == nil {
		return 0
	}
	return session.EffectiveBlocksToProcessBeforeStartBlock + session.EffectiveBlocksToProcessAfterStartBlock
}

// cachedBlocks is the work the server did not have to redo, in module-blocks.
func cachedBlocks(session *pbsubstreamsrpc.SessionInit) uint64 {
	if session == nil {
		return 0
	}

	var cached uint64
	if session.BlocksToProcessBeforeStartBlock > session.EffectiveBlocksToProcessBeforeStartBlock {
		cached += session.BlocksToProcessBeforeStartBlock - session.EffectiveBlocksToProcessBeforeStartBlock
	}
	if session.BlocksToProcessAfterStartBlock > session.EffectiveBlocksToProcessAfterStartBlock {
		cached += session.BlocksToProcessAfterStartBlock - session.EffectiveBlocksToProcessAfterStartBlock
	}
	return cached
}

// parallelTarget is the block at which parallel processing stops. LinearHandoffBlock is the
// accurate answer; ResolvedStartBlock is the fallback for servers that leave it unset.
func parallelTarget(session *pbsubstreamsrpc.SessionInit) uint64 {
	if session == nil {
		return 0
	}
	if session.LinearHandoffBlock != 0 {
		return session.LinearHandoffBlock
	}
	return session.ResolvedStartBlock
}

type stageStat struct {
	Index      int
	Ratio      float64
	Ready      uint64
	SquashWait uint64
	Jobs       int
	OldestJob  time.Duration
}

// computeStageStats derives, for each stage, how far it has actually got and how much job
// pressure sits on it. The span ends at the parallel handoff.
//
// Progress is measured from ready_up_to_exclusive, never from completed_ranges. The server
// fills completed_ranges from segments in state UnitCompleted, UnitPartialPresent *or*
// UnitMerging, so a segment counts there as soon as its partial has been produced — before the
// squasher on tier1 has merged it. Measured that way a stage reaches 100% while real work
// remains, and since squashing runs on tier1 rather than on a worker it schedules no job and
// advances no processed_blocks, so the request reads as frozen at 100% with a rate of zero.
// ready_up_to_exclusive stops at the last squashed segment and does not have that problem.
//
// It also fixes the span anchor. It is the *lowest contiguous* block across the stage's
// modules, so a stage that has not started reports the block its modules begin at: taking it
// as a lower bound keeps the bar honest about the part of the range nothing has touched yet,
// which ranges and jobs alone cannot express.
//
// Zero is a valid value, not "unknown" — a stage whose modules and chain both start at 0
// legitimately reports 0. Never branch on it being zero.
func computeStageStats(stages []*pbsubstreamsrpc.Stage, jobs []*pbsubstreamsrpc.Job, target uint64) []stageStat {
	out := make([]stageStat, len(stages))

	for i, stage := range stages {
		stat := stageStat{
			Index:      i,
			Ready:      stage.ReadyUpToExclusive,
			SquashWait: stage.SquashWaitSegmentCount,
		}

		lo, hasLo := stageLowBlock(stage, jobs, i)
		for _, job := range jobs {
			if int(job.Stage) != i {
				continue
			}
			stat.Jobs++
			if age := time.Duration(job.DurationMs) * time.Millisecond; age > stat.OldestJob {
				stat.OldestJob = age
			}
		}

		if hasLo && target > lo && stat.Ready > lo {
			stat.Ratio = float64(stat.Ready-lo) / float64(target-lo)
			if stat.Ratio > 1 {
				stat.Ratio = 1
			}
		}

		out[i] = stat
	}

	return out
}

func stageLowBlock(stage *pbsubstreamsrpc.Stage, jobs []*pbsubstreamsrpc.Job, stageIndex int) (uint64, bool) {
	var lo uint64
	var found bool

	consider := func(block uint64) {
		if !found || block < lo {
			lo, found = block, true
		}
	}

	// ready_up_to_exclusive first: on a stage that has not started it is the only thing that
	// says where the stage is supposed to begin.
	consider(stage.ReadyUpToExclusive)
	for _, r := range stage.CompletedRanges {
		consider(r.StartBlock)
	}
	for _, job := range jobs {
		if int(job.Stage) == stageIndex {
			consider(job.StartBlock)
		}
	}

	return lo, found
}

type moduleRow struct {
	Stage     int
	Name      string
	Recent    float64
	HasRecent bool
	Avg       float64
	Extra     string
}

// Cost is the figure the row is ranked and filtered on: what the module costs now if we know
// it, what it has cost on average otherwise.
func (r moduleRow) Cost() float64 {
	if r.HasRecent {
		return r.Recent
	}
	return r.Avg
}

// computeModuleRows ranks modules by per-block cost across every stage. The ranking is
// deliberately global rather than per stage: the question the section answers is "what is the
// most expensive thing in this run", and the stage tag on each row keeps the attribution.
func computeModuleRows(progress *pbsubstreamsrpc.ModulesProgress, recent *moduleWindow) []moduleRow {
	if progress == nil {
		return nil
	}

	stageOf := map[string]int{}
	var allNames []string
	for i, stage := range progress.Stages {
		for _, name := range stage.Modules {
			stageOf[name] = i
			allNames = append(allNames, name)
		}
	}
	for _, stats := range progress.ModulesStats {
		if _, found := stageOf[stats.Name]; !found {
			allNames = append(allNames, stats.Name)
		}
	}
	shortNames := shortModuleNames(allNames)

	var rows []moduleRow
	for _, stats := range progress.ModulesStats {
		if stats.TotalProcessedBlockCount == 0 {
			continue
		}

		stage, found := stageOf[stats.Name]
		if !found {
			stage = -1
		}

		row := moduleRow{
			Stage: stage,
			Name:  shortNames[stats.Name],
			Avg:   float64(stats.TotalProcessingTimeMs) / float64(stats.TotalProcessedBlockCount),
			Extra: moduleExtraMetrics(stats),
		}
		if recent != nil {
			row.Recent, row.HasRecent = recent.msPerBlock(stats.Name)
		}

		if row.Cost() < moduleCostFloorMs {
			continue
		}

		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Cost() > rows[j].Cost() })
	if len(rows) > maxModuleRows {
		rows = rows[:maxModuleRows]
	}

	return rows
}

func moduleExtraMetrics(stats *pbsubstreamsrpc.ModuleStats) string {
	if stats.TotalProcessingTimeMs == 0 {
		return ""
	}

	var parts []string
	if stats.TotalStoreOperationTimeMs != 0 {
		blocks := stats.TotalProcessedBlockCount
		if blocks == 0 {
			blocks = 1
		}
		parts = append(parts, fmt.Sprintf("[store (%d read/blk, %d write/blk, %d deletePrefix/blk): %d%%]",
			stats.TotalStoreReadCount/blocks,
			stats.TotalStoreWriteCount/blocks,
			stats.TotalStoreDeleteprefixCount/blocks,
			100*stats.TotalStoreOperationTimeMs/stats.TotalProcessingTimeMs))
	}
	for _, ext := range stats.ExternalCallMetrics {
		parts = append(parts, fmt.Sprintf("[%s (%d): %d%%]", ext.Name, ext.Count, 100*ext.TimeMs/stats.TotalProcessingTimeMs))
	}

	return strings.Join(parts, " ")
}

// progressLines renders the whole backprocessing block. It is the only place that decides
// between the three states the block can be in.
func (m model) progressLines() []string {
	if m.session == nil {
		return []string{"Backprocessing  waiting for session…"}
	}

	total := globalTotal(m.session)
	if total == 0 {
		if cached := cachedBlocks(m.session); cached != 0 {
			return []string{fmt.Sprintf("Backprocessing  nothing to do  —  %s blocks already cached", formatx.BlockNumber(cached))}
		}
		return []string{"Backprocessing  nothing to do"}
	}

	if m.progress == nil {
		return []string{fmt.Sprintf("Backprocessing  starting…  %s blocks to process", formatx.Count(total))}
	}

	done := globalDone(m.progress)
	if done >= total {
		return []string{fmt.Sprintf("Backprocessing  complete  ·  %s blocks in %s", formatx.Count(total), formatx.Duration(m.elapsed()))}
	}

	lines := []string{m.headerLine(done, total)}
	lines = append(lines, m.stageLines()...)
	lines = append(lines, m.moduleLines()...)

	return lines
}

func (m model) elapsed() time.Duration {
	if m.sessionAt.IsZero() {
		return 0
	}
	return m.now().Sub(m.sessionAt)
}

func (m model) headerLine(done, total uint64) string {
	ratio := float64(done) / float64(total)

	var segments []string

	rate, span, ok := m.globalRate.perSecond()
	if ok {
		segments = append(segments, formatx.Rate(rate, "blk"))

		// An ETA computed off a window that barely spans two samples swings wildly enough to
		// be worse than no ETA at all.
		if span >= minWindowForRate && rate > 0 {
			segments = append(segments, "eta "+formatx.Duration(time.Duration(float64(total-done)/rate)*time.Second))
		}
	}

	if jobs := len(m.progress.RunningJobs); m.session.MaxParallelWorkers != 0 {
		segments = append(segments, fmt.Sprintf("%d/%d jobs", jobs, m.session.MaxParallelWorkers))
	} else if jobs != 0 {
		segments = append(segments, fmt.Sprintf("%d jobs", jobs))
	}

	if cached := cachedBlocks(m.session); cached != 0 {
		segments = append(segments, fmt.Sprintf("(%s cached)", formatx.Count(cached)))
	}

	return progressRow("Backprocessing", ratio, barWidthFor(m.Width), segments...)
}

func (m model) stageLines() []string {
	stages := m.progress.Stages
	if len(stages) == 0 {
		return nil
	}

	target := parallelTarget(m.session)
	stats := computeStageStats(stages, m.progress.RunningJobs, target)
	width := barWidthFor(m.Width)

	var lines []string

	// Beyond maxStageRows, the finished stages at the front are the least interesting thing on
	// screen: collapse them into one row and spend the remaining rows on the active ones.
	visible := stats
	if len(stats) > maxStageRows {
		collapsed := 0
		for collapsed < len(stats) && stats[collapsed].Ratio >= 1 {
			collapsed++
		}
		if maxCollapsible := len(stats) - maxStageRows; collapsed > maxCollapsible {
			collapsed = maxCollapsible
		}
		if collapsed == 1 {
			// A single stage does not earn a "collapsed" row, it can just show itself.
			collapsed = 0
		}
		if collapsed > 0 {
			lines = append(lines, progressRow(stageLabel(fmt.Sprintf("s0-s%d", collapsed-1)), 1, width))
			visible = stats[collapsed:]
		}

		// Whatever is left over is unfinished, and the frontier says more about the run than its
		// head does. Drop from the front, and say so rather than truncating silently.
		if dropped := len(visible) - maxStageRows; dropped > 0 {
			visible = visible[dropped:]
			lines = append(lines, fmt.Sprintf("%s  %d more %s not shown", padLabel(stageLabel("…")), dropped, pluralize(dropped, "stage")))
		}
	}

	for _, stat := range visible {
		var details []string
		if stat.Jobs != 0 {
			details = append(details, fmt.Sprintf("%s, oldest %s", pluralJobs(stat.Jobs), formatx.Duration(stat.OldestJob)))
		}
		// Named explicitly because it is the state that used to read as a stall: tier1 is
		// working, but no job is running and no block counter is moving.
		if stat.SquashWait != 0 {
			details = append(details, fmt.Sprintf("squashing %d %s", stat.SquashWait, pluralize(int(stat.SquashWait), "segment")))
		}
		lines = append(lines, progressRow(stageLabel(fmt.Sprintf("s%d", stat.Index)), stat.Ratio, width, details...))
	}

	if out := m.outLine(width); out != "" {
		// With a single stage the out row would restate that stage's own bar, so its frontier
		// is appended to it instead.
		if len(stages) == 1 && len(lines) == 1 {
			lines[0] += "  ·  " + out
		} else {
			lines = append(lines, out)
		}
	}

	return lines
}

func pluralJobs(n int) string {
	if n == 1 {
		return "1 job"
	}
	return fmt.Sprintf("%d jobs", n)
}

// outLine reports where the output module has reached, in chain blocks, against the block the
// client asked to start at. It answers "when does data start flowing", which none of the
// module-block figures above it can.
func (m model) outLine(width int) string {
	stages := m.progress.Stages
	if len(stages) == 0 {
		return ""
	}

	last := stages[len(stages)-1]
	lo, hasLo := stageLowBlock(last, m.progress.RunningJobs, len(stages)-1)
	if !hasLo {
		// The last stage has not reported anything yet, which is the normal state early in a
		// run: earlier stages have to produce before it can. Anchoring on the lowest block any
		// stage knows about keeps the row on screen at 0% instead of having it appear from
		// nowhere later — a live region that changes shape is harder to read than a still one.
		for i, stage := range stages {
			if stageLo, found := stageLowBlock(stage, m.progress.RunningJobs, i); found && (!hasLo || stageLo < lo) {
				lo, hasLo = stageLo, true
			}
		}
	}
	if !hasLo {
		return ""
	}

	target := m.session.ResolvedStartBlock
	if target <= lo {
		return ""
	}

	// The frontier is the last stage's squashed readiness, not the end of its produced ranges:
	// output can only be served from what has actually been squashed.
	frontier := last.ReadyUpToExclusive
	if frontier < lo {
		frontier = lo
	}
	if frontier > target {
		frontier = target
	}
	ratio := float64(frontier-lo) / float64(target-lo)

	if len(stages) == 1 {
		return fmt.Sprintf("%s → %s", formatx.BlockNumber(frontier), formatx.BlockNumber(target))
	}

	return progressRow(stageLabel("out"), ratio, width,
		fmt.Sprintf("%s → %s", formatx.BlockNumber(frontier), formatx.BlockNumber(target)))
}

func (m model) moduleLines() []string {
	rows := computeModuleRows(m.progress, m.moduleRates)
	if len(rows) == 0 {
		return nil
	}

	// Both time bases are shown because they answer different questions: the recent cost is
	// what the module is doing now, the average is what it has cost over the whole run.
	costs := make([]string, len(rows))
	nameWidth, costWidth := 0, 0
	for i, row := range rows {
		costs[i] = costPerBlock(row.Avg)
		if row.HasRecent {
			costs[i] = fmt.Sprintf("%s (avg %s)", costPerBlock(row.Recent), formatx.Millis(row.Avg))
		}

		if len(row.Name) > nameWidth {
			nameWidth = len(row.Name)
		}
		if len(costs[i]) > costWidth {
			costWidth = len(costs[i])
		}
	}

	lines := []string{"", "Slowest modules"}
	for i, row := range rows {
		stage := "  "
		if row.Stage >= 0 {
			stage = fmt.Sprintf("s%d", row.Stage)
		}

		lines = append(lines, strings.TrimRight(fmt.Sprintf("  %s  %-*s  %-*s  %s", stage, nameWidth, row.Name, costWidth, costs[i], row.Extra), " "))
	}

	return lines
}
