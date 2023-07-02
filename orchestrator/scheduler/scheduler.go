package scheduler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

type Scheduler struct {
	ctx context.Context
	loop.EventLoop

	stream *response.Stream

	Stages        *stage.Stages
	WorkerPool    *work.WorkerPool
	ExecOutWalker *execout.Walker

	logger *zap.Logger

	// Final state:
	outputStreamCompleted bool
	storesSyncCompleted   bool
}

func New(ctx context.Context, stream *response.Stream) *Scheduler {
	logger := reqctx.Logger(ctx)
	s := &Scheduler{
		ctx:    ctx,
		stream: stream,
		logger: logger,
	}
	s.EventLoop = loop.NewEventLoop(s.Update)
	return s
}

func (s *Scheduler) Init() loop.Cmd {
	var cmds []loop.Cmd

	if s.ExecOutWalker != nil {
		cmds = append(cmds, execout.CmdMsgStartDownload())
	} else {
		// This hides the fact that there _was no_ Walker. Could cause
		// confusing error messages in `cmdShutdownWhenComplete()`.
		s.outputStreamCompleted = true
	}
	cmds = append(cmds, work.CmdScheduleNextJob())

	cmds = append(cmds, s.Stages.CmdStartMerge())

	return loop.Batch(cmds...)
}

func (s *Scheduler) Update(msg loop.Msg) loop.Cmd {
	fmt.Printf("Scheduler message: %T %v\n", msg, msg)
	fmt.Print(s.Stages.StatesString())
	var cmds []loop.Cmd

	switch msg := msg.(type) {
	case work.MsgJobSucceeded:
		s.Stages.MarkSegmentPartialPresent(msg.Unit)
		s.WorkerPool.Return(msg.Worker)
		cmds = append(cmds,
			s.Stages.CmdTryMerge(msg.Unit.Stage),
			work.CmdScheduleNextJob(),
		)

	case work.MsgScheduleNextJob:

		workUnit, workRange := s.Stages.NextJob()
		if workRange == nil {
			return nil
		}

		if !s.WorkerPool.WorkerAvailable() {
			return nil
		}
		worker := s.WorkerPool.Borrow()

		s.logger.Info("scheduling work", zap.Object("unit", workUnit))
		return loop.Batch(
			worker.Work(s.ctx, workUnit, workRange, s.stream),
			work.CmdScheduleNextJob(),
		)

	case work.MsgJobFailed:
		cmds = append(cmds, loop.Quit(msg.Error))

	//case stage.MsgDetectedNewPartial:
	//	// When we have a polling thing for partials (or etcd instructions
	//	// to the effect that some partial jobs were produced elsewhere)
	//	// Again, that other place would need to detect _all stores_ for the
	//	// given Unit.
	//	if s.Stages.DetectedPartial(msg.Unit) {
	//		// cancel running job,
	//		// if the Unit was Scheduled,
	//		// then _cancel the job_ somehow
	//		// and mark it PartialPresent.
	//		// if it was Pending, then simply schedule the next job
	//		// and some merging work.
	//	}

	case stage.MsgMergeFinished:
		s.Stages.MergeCompleted(msg.Unit)
		cmds = append(cmds, s.Stages.CmdTryMerge(msg.Stage))

	case stage.MsgMergeStoresCompleted:
		s.storesSyncCompleted = true
		return s.cmdShutdownWhenComplete()

	case stage.MsgMergeFailed:
		cmds = append(cmds, loop.Quit(msg.Error))

	case execout.MsgStartDownload:
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(0))

	case execout.MsgFileNotPresent:
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(2*time.Second))

	case execout.MsgFileDownloaded:
		s.ExecOutWalker.NextSegment()
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(0))

	case execout.MsgWalkerCompleted:
		s.outputStreamCompleted = true
		return s.cmdShutdownWhenComplete()

	}

	if len(cmds) != 0 {
		fmt.Printf("Schedule: %T %v\n", cmds, cmds)
	}
	return loop.Batch(cmds...)
}

func (s *Scheduler) cmdShutdownWhenComplete() loop.Cmd {
	if s.outputStreamCompleted && s.storesSyncCompleted {
		return func() loop.Msg {
			err := s.Stages.WaitAsyncWork()
			return loop.Quit(err)()
		}
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

func (s *Scheduler) FinalStoreMap(exclusiveEndBlock uint64) (store.Map, error) {
	return s.Stages.FinalStoreMap(exclusiveEndBlock)
}
