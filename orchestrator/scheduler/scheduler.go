package scheduler

import (
	"context"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/responses"
	"github.com/streamingfast/substreams/orchestrator/squasher"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
)

type Scheduler struct {
	ctx context.Context
	loop.EventLoop

	stream                 *responses.Stream
	upstreamRequestModules *pbsubstreams.Modules

	Planner    *work.Plan
	Squasher   *squasher.Multi
	WorkerPool work.WorkerPool

	// Status:
	JobStatus    []*work.Job
	WorkerStatus map[string]string

	stagedModules StagedModules

	// Output
	outputStoreMap store.Map
}

func New(ctx context.Context, stream *responses.Stream, upstreamRequestModules *pbsubstreams.Modules) *Scheduler {
	s := &Scheduler{
		ctx:                    ctx,
		stream:                 stream,
		upstreamRequestModules: upstreamRequestModules,
	}
	s.EventLoop = loop.NewEventLoop(ctx, s.Update)
	return s
}

func (s *Scheduler) Update(msg loop.Msg) loop.Cmd {
	var cmds []loop.Cmd

	switch msg := msg.(type) {
	case JobStartedMsg:
	case JobFailedMsg:
		s.updateJobFailed(msg)
		return ScheduleNextJobMsg()
	case JobSucceededMsg:
		s.jobs[msg.JobID].Finished = true
		s.stages[msg.Stage].segments[msg.Segment].Completed = true
		return s.checkFullStoresPresence(stage, segment)
	case JobFinishedMsg:
		return loop.Batch(
			s.mergePartial(msg.Range),
			ScheduleNextJob(),
		)

	case StoragePartialFoundMsg:
		s.killPotentiallyRunningJob(msg.JobID)
		s.mergePartial(msg.Range)
		return ScheduleNextJob()

	case MergeStartedMsg:
	case MergeFinishedMsg:
	case MergeFailedMsg:

	case WorkerAvailableMsg:
		s.workers.AppendWaiting(msg.worker)
		return ScheduleNextJobMsg()

	case ScheduleNextJobMsg:
		job := s.Planner.NextJob()
		if job == nil {
			return nil
		}
		if len(s.WorkerPool.PendingCount()) == 0 {
			return nil
		}
		worker := s.workers.Borrow()
		return s.runJob(worker, msg.job)

	case FullStoresPresent:
		stage := s.stages[msg.Stage]
		sq := s.Squasher[msg.Stage]

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
	}

	return loop.Batch(cmds...)
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
	return loop.Sequence(
		MergeStartedMsg(),
		func() loop.Msg {

		},
	)
}

func (s *Scheduler) FinalStoreMap() store.Map {
	return s.outputStoreMap
}
