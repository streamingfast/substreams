package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/squasher"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

type Scheduler struct {
	ctx context.Context
	loop.EventLoop

	stream      *response.Stream
	outputGraph *outputmodules.Graph

	Stages        *stage.Stages
	Squasher      *squasher.Multi
	WorkerPool    *work.WorkerPool
	ExecOutWalker *execout.Walker

	// Status:
	//JobStatus    []*work.Job
	//WorkerStatus map[string]string

	logger *zap.Logger

	// Final state:
	outputStreamCompleted bool
	storesSyncCompleted   bool
}

func New(ctx context.Context, stream *response.Stream, outputGraph *outputmodules.Graph) *Scheduler {
	logger := reqctx.Logger(ctx)
	s := &Scheduler{
		ctx:         ctx,
		stream:      stream,
		outputGraph: outputGraph, // upstreamRequestModules is replaced by outputGraph.UsedModules(), UNLESS the consumer wanted ALL the Requested modules.. even those who are not necessary to satisfy this request (that would be.. waste)
		logger:      logger,
	}
	s.EventLoop = loop.NewEventLoop(s.Update)
	return s
}

func (s *Scheduler) Init() {
	// TODO: Kickstart the Jobs processing

	if s.ExecOutWalker != nil {
		s.Send(execout.MsgStartDownload{})
	} else {
		// This hides the fact that there was _no_ Walker. Could cause
		// confusing error messages in `cmdShutdownWhenComplete()`.
		s.outputStreamCompleted = true
	}
}

func (s *Scheduler) Update(msg loop.Msg) loop.Cmd {
	var cmds []loop.Cmd

	switch msg := msg.(type) {
	case work.MsgJobFailed:
		cmds = append(cmds, loop.Quit(msg.Error))

	case work.MsgJobSucceeded:
		//s.JobStatus.MarkFinished(msg.JobID)
		s.Stages.MarkSegmentPartialPresent(msg.Stage, msg.Segment)
		cmds = append(cmds,
			s.Squasher.AddPartials(msg.Files...),
			work.CmdScheduleNextJob(),
		)

	case work.MsgWorkerFreed:
		s.WorkerPool.Return(msg.Worker)
		return work.CmdScheduleNextJob()

	case work.MsgScheduleNextJob:
		jobSegment := s.Stages.NextJob()
		if jobSegment == nil {
			return nil
		}

		if !s.WorkerPool.WorkerAvailable() {
			return nil
		}
		worker := s.WorkerPool.Borrow()

		return loop.Batch(
			worker.Work(s.ctx, *jobSegment, s.stream),
			work.CmdScheduleNextJob(),
		)

	case MsgStoragePartialFound:
		cmds = append(cmds, s.continueMergingWork())
		job := s.killPotentiallyRunningJob(msg.JobID)
		if job != nil {
			cmds = append(cmds, work.CmdScheduleNextJob())
		}

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
		return s.Squasher.MarkSingleFinished(msg)
		if allStoresAreCompletedUpToTargetBlock() {
			s.storesSyncCompleted = true
			return s.cmdShutdownWhenComplete()
		}

	case squasher.MsgMergeFailed:

	case squasher.MsgMergeStage:
		for _, mod := range s.outputGraph.StagedUsedModules()[msg.Stage] {
			cmds = append(cmds, s.Squasher.MergeNextRange(mod.Name)) //Modules[mod.Name].CmdMergeRange())
		}
	//sq := s.squashers[msg.Stage]
	//for _, storeSquasher := range sq.Stores {
	//	if mergeRange := storeSquasher.NextRangeToMerge(); mergeRange != nil {
	//		cmds = append(cmds, storeSquasher.CmdMergeRange(mergeRange))
	//	}
	//}

	case execout.MsgStartDownload:
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(0))
	case execout.MsgFileNotPresent:
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(2*time.Second))
	case execout.MsgFileDownloaded:
		if s.ExecOutWalker.IsCompleted() {
			s.outputStreamCompleted = true
			return s.cmdShutdownWhenComplete()
		}
		s.ExecOutWalker.NextSegment()
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(0))
	}

	return loop.Batch(cmds...)
}

func (s *Scheduler) runJob(job *work.Job, stage string) loop.Cmd {
	return job.CmdRun()
}

func (s *Scheduler) continueMergingWork() loop.Cmd {
	// Check with the Squasher where we're at
	// Check the Squasher's merging state for this Stage
	// Check where it is at for each Stage, what's the next Segment
	// Check with the `Stages` if that segment is in PartialPresent
	// If so, start the merging operation
	// Change the stages.MarkSegmentMerging(segment, stage)
}

func (s *Scheduler) cmdShutdownWhenComplete() loop.Cmd {
	// TODO: ensure everything else is completed properly,
	// like the setting of the stores and all, only then do you
	// Quit.
	// Anything that could cause the thing to complete should call
	// cmdShutdownWhenComplete()
	if s.outputStreamCompleted && s.storesSyncCompleted {
		return loop.Quit(nil)
	}
	if !s.outputStreamCompleted && !s.storesSyncCompleted {
		s.logger.Info("scheduler: waiting for output stream and stores to complete")
	}
	if !s.outputStreamCompleted && s.storesSyncCompleted {
		s.logger.Info("scheduler: waiting for output stream to complete, stores ready")
	}
	if s.outputStreamCompleted && !s.storesSyncCompleted {
		s.logger.Info("scheduler: waiting for stores to complete, output stream completed")
	}
	return nil

}

func (s *Scheduler) FinalStoreMap() store.Map {
	return s.Squasher.FinalStoreMap()
}
