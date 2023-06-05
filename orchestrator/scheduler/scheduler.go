package scheduler

import (
	"context"

	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/responses"
	"github.com/streamingfast/substreams/orchestrator/squasher"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/storage/store"
)

type Scheduler struct {
	ctx context.Context
	loop.EventLoop

	stream      *responses.Stream
	outputGraph *outputmodules.Graph

	Planner    *work.Plan
	Squasher   *squasher.Multi
	WorkerPool *work.WorkerPool

	// Status:
	JobStatus    []*work.Job
	WorkerStatus map[string]string

	Stages stage.Stages
}

func New(ctx context.Context, stream *responses.Stream, outputGraph *outputmodules.Graph) *Scheduler {
	s := &Scheduler{
		ctx:         ctx,
		stream:      stream,
		outputGraph: outputGraph, // upstreamRequestModules is replaced by outputGraph.UsedModules(), UNLESS the consumer wanted ALL the Requested modules.. even those who are not necessary to satisfy this request (that would be.. waste)
	}
	s.EventLoop = loop.NewEventLoop(s.Update)
	s.init()
	return s
}

func (s *Scheduler) init() {
	// create the `stagedModules` based on the `Modules`
	// and the desired output module.
	// Launch the command to fetch the first state on disk
	//   and a Message saying we have all the storage snapshots
	//   ready.
	// Initialize the store.Map
	// Kickstart the Jobs processing
}

func (s *Scheduler) Update(msg loop.Msg) loop.Cmd {
	var cmds []loop.Cmd

	switch msg := msg.(type) {
	// INTERACTIONS WITH JOB PROCESSING

	case work.MsgJobStarted:
	case work.MsgJobFailed:
		// TODO: When job fails, do we not quit??
		//  The retry loop is within the job execution, so we wouldn't redo it here
		s.JobStatus.MarkFailed(msg)
		return work.CmdScheduleNextJob()
	case work.MsgJobSucceeded:
		s.JobStatus.MarkFinished(msg.JobID)
		s.Stages.MarkSegmentCompleted(msg.Stage, msg.Segment)
		return s.checkFullStoresPresence(stage, segment)
		// or:
		//return loop.Batch(
		//	s.mergePartial(msg.Range),
		//	ScheduleNextJob(),
		//)

	case MsgStoragePartialFound:
		s.killPotentiallyRunningJob(msg.JobID)
		s.mergePartial(msg.Range)
		return work.CmdScheduleNextJob()

	case work.MsgWorkerFreed:
		s.WorkerPool.Return(msg.Worker)
		return work.CmdScheduleNextJob()

	case work.MsgScheduleNextJob:
		job := s.Planner.NextJob()
		if job == nil {
			return nil
		}
		if !s.WorkerPool.WorkerAvailable() {
			return nil
		}
		worker := s.WorkerPool.Borrow()
		return loop.Batch(
			s.runJob(worker, msg.job),
			work.CmdScheduleNextJob(),
		)

	case FullStoresPresent:
		stage := s.Stages[msg.Stage]
		sq := s.Squasher[msg.Stage]

		if len(msg.Stores) != len(stage.Stores) {
			return nil
		}

		return loop.Batch(
			s.killPotentiallyRunningJob(msg.Stage, msg.Segment),
			loop.Sequence(
				s.Squasher.AddPartials(msg.StoreName, msg.Files...),
				s.mergeStage(msg.Stage),
			),
		)

	case squasher.MsgMergeStarted:
	case squasher.MsgMergeFinished:
	case squasher.MsgMergeFailed:

	case squasher.MsgMergeStage:
		for _, mod := range s.outputGraph.StagedUsedModules()[msg.Stage] {
			cmds = append(cmds, s.Squasher.MergeNextRange(mod.Name)) //Modules[mod.Name].MergeNextRange())
		}
		//sq := s.squashers[msg.Stage]
		//for _, storeSquasher := range sq.Stores {
		//	if mergeRange := storeSquasher.NextRangeToMerge(); mergeRange != nil {
		//		cmds = append(cmds, storeSquasher.MergeRange(mergeRange))
		//	}
		//}
	}

	return loop.Batch(cmds...)
}

func (s *Scheduler) FinalStoreMap() store.Map {
	return s.Squasher.FinalStoreMap()
}
