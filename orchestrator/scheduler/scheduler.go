package scheduler

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/metrics"
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
		cmds = append(cmds, execout.CmdDownloadSegment(0))
	} else {
		// This hides the fact that there _was no_ Walker. Could cause
		// confusing error messages in `cmdShutdownWhenComplete()`.
		s.outputStreamCompleted = true
	}

	cmds = append(cmds, work.CmdScheduleNextJob())

	if s.Stages.AllStoresCompleted() {
		cmds = append(cmds, func() loop.Msg { return stage.MsgAllStoresCompleted{} })
	}

	cmds = append(cmds, s.Stages.CmdStartMerge())

	return loop.Batch(cmds...)
}

func (s *Scheduler) Update(msg loop.Msg) loop.Cmd {
	defer s.Stages.UpdateStats()

	if os.Getenv("SUBSTREAMS_DEBUG_SCHEDULER_STATE") == "true" {
		fmt.Print(s.Stages.StatesString())
		fmt.Printf("Scheduler message: %T %v\n", msg, msg)
	}
	//cmd, _ := exec.Command("bash", "-c", "cd "+os.Getenv("TEST_TEMP_DIR")+"; find .").Output()
	//fmt.Print(string(cmd))
	var cmds []loop.Cmd

	switch msg := msg.(type) {
	case work.MsgJobSucceeded:
		metrics.Tier1ActiveWorkerRequest.Dec()

		s.Stages.MarkSegmentPartialPresent(msg.Unit)
		s.WorkerPool.Return(msg.Worker)

		cmds = append(cmds,
			s.Stages.CmdTryMerge(msg.Unit.Stage),
			work.CmdScheduleNextJob(),
		)
		if s.ExecOutWalker != nil {
			cmds = append(cmds, execout.CmdDownloadSegment(0))
		}

	case work.MsgScheduleNextJob:
		avail, shouldRetry := s.WorkerPool.WorkerAvailable()
		if !avail {
			if !shouldRetry {
				return nil
			}
			cmds = append(cmds, loop.Tick(time.Second, func() loop.Msg { return work.MsgScheduleNextJob{} }))
			break
		}
		workUnit, workRange := s.Stages.NextJob()
		if workRange == nil {
			return nil
		}

		worker := s.WorkerPool.Borrow()

		s.logger.Info("scheduling work", zap.Object("unit", workUnit))
		modules := s.Stages.StageModules(workUnit.Stage)

		metrics.Tier1ActiveWorkerRequest.Inc()
		metrics.Tier1WorkerRequestCounter.Inc()

		return loop.Batch(
			worker.Work(s.ctx, workUnit, workRange, modules, s.stream),
			work.CmdScheduleNextJob(),
		)

	case work.MsgJobFailed:
		metrics.Tier1ActiveWorkerRequest.Dec()

		cmds = append(cmds, loop.Quit(msg.Error))

	case stage.MsgMergeFinished:
		s.Stages.MergeCompleted(msg.Unit)
		cmds = append(cmds,
			work.CmdScheduleNextJob(),
			s.Stages.CmdTryMerge(msg.Stage),
		)

	case stage.MsgAllStoresCompleted:
		s.storesSyncCompleted = true
		cmds = append(cmds,
			work.CmdScheduleNextJob(), // in case some mapper jobs need scheduling
			s.cmdShutdownWhenComplete(),
		)

	case stage.MsgMergeFailed:
		cmds = append(cmds, loop.Quit(msg.Error))

	case execout.MsgFileNotPresent:
		s.ExecOutWalker.MarkNotWorking()
		cmds = append(cmds, execout.CmdDownloadSegment(msg.NextWait))

	case execout.MsgFileDownloaded:
		s.ExecOutWalker.NextSegment()
		s.ExecOutWalker.MarkNotWorking()
		cmds = append(cmds, execout.CmdDownloadSegment(0))

	case execout.MsgDownloadSegment:
		if s.ExecOutWalker == nil {
			return nil
		}
		if s.ExecOutWalker.IsWorking() {
			return nil
		}
		s.ExecOutWalker.MarkWorking()
		if s.ExecOutWalker.IsCompleted() {
			return execout.CmdWalkerCompleted()
		}
		cmds = append(cmds, s.ExecOutWalker.CmdDownloadCurrentSegment(msg.Wait))

	case execout.MsgWalkerCompleted:
		s.outputStreamCompleted = true
		return s.cmdShutdownWhenComplete()

	}

	return loop.Batch(cmds...)
}

func (s *Scheduler) cmdShutdownWhenComplete() loop.Cmd {
	if s.outputStreamCompleted && s.storesSyncCompleted {

		var fields []zap.Field
		if s.ExecOutWalker != nil {
			start, current, end := s.ExecOutWalker.Progress()
			fields = append(fields, zap.Int("cached_output_start", start), zap.Int("cached_output_current", current), zap.Int("cached_output_end", end))
		}
		s.logger.Info("scheduler: stores and cached_outputs stream completed, switching to live", fields...)
		return func() loop.Msg {
			err := s.Stages.WaitAsyncWork()
			return loop.Quit(err)()
		}
	}
	if !s.outputStreamCompleted && !s.storesSyncCompleted {
		s.logger.Info("scheduler: waiting for output stream and stores to complete")
	}
	if !s.outputStreamCompleted && s.storesSyncCompleted {

		var fields []zap.Field
		if s.ExecOutWalker != nil {
			start, current, end := s.ExecOutWalker.Progress()
			fields = append(fields, zap.Int("cached_output_start", start), zap.Int("cached_output_current", current), zap.Int("cached_output_end", end))
		}
		s.logger.Info("scheduler: waiting for output stream to complete, stores ready", fields...)
	}
	if s.outputStreamCompleted && !s.storesSyncCompleted {
		s.logger.Info("scheduler: waiting for stores to complete, output stream completed")
	}
	return nil

}

func (s *Scheduler) FinalStoreMap(exclusiveEndBlock uint64) (store.Map, error) {
	return s.Stages.FinalStoreMap(exclusiveEndBlock)
}
