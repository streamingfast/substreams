package scheduler

import (
	"context"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/squasher"
	"github.com/streamingfast/substreams/orchestrator/work"
)

type Scheduler struct {
	ctx       context.Context
	EventLoop loop.EventLoop

	Planner    *work.Plan
	Squasher   *squasher.Multi
	RunnerPool work.WorkerPool

	// Status:
	JobStatus    []*work.Job
	WorkerStatus map[string]string

	stagedModules StagedModules
}

func New(ctx context.Context) *Scheduler {
	s := &Scheduler{
		ctx: ctx,
	}
	s.EventLoop = loop.NewEventLoop(ctx, s.Update)
	return s
}

func (s *Scheduler) Update(msg loop.Msg) loop.Cmd {
	switch msg := msg.(type) {
	case work.JobFinished:
		return loop.Batch(
			s.mergePartial(msg.Range),
			ScheduleNextJobMsg(),
		)
	case ObservedPartialReadyMsg:
		s.killPotentiallyRunningJob(msg.JobID)
		s.mergePartial(msg.Range)
		return ScheduleNextJobMsg()
	case work.JobFailed:
		s.updateJobFailed(msg)
		return ScheduleNextJobMsg()

	case work.JobStarted:
	case MergeStarted:
	case MergeFinished:
	case MergeFailed:

	case WorkerAvailable:
		s.workers.AppendWaiting(msg.worker)
		return ScheduleNextJobMsg()

	case ScheduleNextJob:
		job := s.nextJob()
		if job == nil {
			return nil
		}
		if len(s.workers.PendingCount()) == 0 {
			return nil
		}
		worker := s.workers.Borrow()
		return s.runJob(worker, msg.job)

	case JobSucceeded:
		s.jobs[msg.JobID].Finished = true
		s.stages[msg.Stage].segments[msg.Segment].Completed = true
		return s.checkFullStoresPresence(stage, segment)

	case FullStoresPresent:
		stage := s.stages[msg.Stage]
		sq := s.squashers[msg.Stage]

		if len(msg.Stores) != len(stage.Stores) {
			return nil
		}

		sq.addFullStoresPresent(msg.Segment, msg.Stores)

		return loop.Batch(
			s.killPotentiallyRunningJob(msg.Stage, msg.Segment),
			s.mergeStage(msg.Stage),
		)
	case MergeStage:
		sq := s.squashers[msg.Stage]
		for _, storeSquasher := range sq.Stores {
			if mergeRange := storeSquasher.NextRangeToMerge(); mergeRange != nil {
				cmds = append(cmds, storeSquasher.MergeRange(mergeRange))
			}
		}
		return loop.Batch(cmds...)
	}
}

func (s *storeSquasher) NextRangeToMerge() *block.Range {
	if s.Status != Waiting {
		return nil
	}
	// TODO: compute whether the store Squasher has some things that are
	// ready and contiguous, in which case we return
	// the Range.
}

func (s *storeSquasher) MergeRange(r *block.Range) loop.Cmd {
	return loop.Sequence{
		MergeStartedMsg(),
		func() Msg {

		},
	}
}

func (s *Scheduler) FinalStoreMap() map[string]Store {
	return s.stores
}

type StagedModules []*StageProgress // staged, alphanumerically sorted module names

type StageProgress struct {
	modules []*ModuleProgress
}

type ModuleProgress struct {
	name string

	readyUpToBlock     uint64
	mergedUpToBlock    uint64
	scheduledUpToBlock uint64

	completedJobs []*work.Job
	runningJobs   []*work.Job
}

// Algorithm for planning the Next Jobs:
// We need to start from the last stage, first segment.
