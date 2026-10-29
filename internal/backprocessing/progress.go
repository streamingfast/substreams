// Package backprocessing derives backprocessing progress figures (overall completion, per-stage
// readiness, output frontier) from the SessionInit and ModulesProgress messages. It is shared by
// the `substreams run` TUI and the sink stats logs so both report the same numbers.
package backprocessing

import (
	"time"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// Done is the amount of work completed, in module-blocks.
//
// ProcessedBlocks alone is a staircase: it only advances when a job completes. Adding the
// progress of the jobs currently in flight makes it advance continuously, which is what
// keeps the rate and the ETA meaningful.
func Done(progress *pbsubstreamsrpc.ModulesProgress) uint64 {
	if progress == nil {
		return 0
	}

	done := progress.ProcessedBlocks
	for _, job := range progress.RunningJobs {
		done += job.ProgressBlocks
	}
	return done
}

// Total is the work left to do at the time the session started, in module-blocks,
// excluding whatever was already cached.
func Total(session *pbsubstreamsrpc.SessionInit) uint64 {
	if session == nil {
		return 0
	}
	return session.EffectiveBlocksToProcessBeforeStartBlock + session.EffectiveBlocksToProcessAfterStartBlock
}

// Cached is the work the server did not have to redo, in module-blocks.
func Cached(session *pbsubstreamsrpc.SessionInit) uint64 {
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

// ParallelTarget is the block at which parallel processing stops. LinearHandoffBlock is the
// accurate answer; ResolvedStartBlock is the fallback for servers that leave it unset.
func ParallelTarget(session *pbsubstreamsrpc.SessionInit) uint64 {
	if session == nil {
		return 0
	}
	if session.LinearHandoffBlock != 0 {
		return session.LinearHandoffBlock
	}
	return session.ResolvedStartBlock
}

// ETA is the time left to process remaining module-blocks at rate, or false when the rate is
// not trustworthy yet.
func ETA(remaining uint64, rate float64, span time.Duration) (time.Duration, bool) {
	// An ETA computed off a window that barely spans two samples swings wildly enough to
	// be worse than no ETA at all.
	if span < MinWindowForRate || rate <= 0 {
		return 0, false
	}
	return time.Duration(float64(remaining)/rate) * time.Second, true
}

type StageStat struct {
	Index int
	Ratio float64
	// Low is the block Ratio is measured from: where the stage begins. Ready equals Low until
	// the stage has made progress, so it cannot tell "starts here" from "done up to here".
	Low        uint64
	Ready      uint64
	SquashWait uint64
	Jobs       int
	OldestJob  time.Duration
}

// StageStats derives, for each stage, how far it has actually got and how much job
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
func StageStats(stages []*pbsubstreamsrpc.Stage, jobs []*pbsubstreamsrpc.Job, target uint64) []StageStat {
	out := make([]StageStat, len(stages))

	for i, stage := range stages {
		stat := StageStat{
			Index:      i,
			Ready:      stage.ReadyUpToExclusive,
			SquashWait: stage.SquashWaitSegmentCount,
		}

		lo, hasLo := StageLowBlock(stage, jobs, i)
		stat.Low = lo
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

func StageLowBlock(stage *pbsubstreamsrpc.Stage, jobs []*pbsubstreamsrpc.Job, stageIndex int) (uint64, bool) {
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

// OutFrontier reports where the output module has reached, in chain blocks, against the block
// the client asked to start at (target). It answers "when does data start flowing", which none
// of the module-block figures can. ok is false when there is nothing meaningful to report.
func OutFrontier(session *pbsubstreamsrpc.SessionInit, progress *pbsubstreamsrpc.ModulesProgress) (lo, frontier, target uint64, ok bool) {
	if session == nil || progress == nil || len(progress.Stages) == 0 {
		return 0, 0, 0, false
	}
	stages := progress.Stages

	last := stages[len(stages)-1]
	lo, hasLo := StageLowBlock(last, progress.RunningJobs, len(stages)-1)
	if !hasLo {
		// The last stage has not reported anything yet, which is the normal state early in a
		// run: earlier stages have to produce before it can. Anchoring on the lowest block any
		// stage knows about reports it at 0% instead of having it appear from nowhere later.
		for i, stage := range stages {
			if stageLo, found := StageLowBlock(stage, progress.RunningJobs, i); found && (!hasLo || stageLo < lo) {
				lo, hasLo = stageLo, true
			}
		}
	}
	if !hasLo {
		return 0, 0, 0, false
	}

	target = session.ResolvedStartBlock
	if target <= lo {
		return 0, 0, 0, false
	}

	// The frontier is the last stage's squashed readiness, not the end of its produced ranges:
	// output can only be served from what has actually been squashed.
	frontier = last.ReadyUpToExclusive
	if frontier < lo {
		frontier = lo
	}
	if frontier > target {
		frontier = target
	}

	return lo, frontier, target, true
}
