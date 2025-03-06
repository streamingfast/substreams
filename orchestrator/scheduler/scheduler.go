package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	WorkerPool    work.WorkerPool
	ExecOutWalker *execout.Walker

	logger *zap.Logger

	// Final state:
	outputStreamCompleted bool
	storesSyncCompleted   bool

	delayedScheduleNextJob bool
}

func New(ctx context.Context, stream *response.Stream) *Scheduler {
	logger := reqctx.Logger(ctx)
	logger = logger.Named("scheduler").With(zap.Bool("keep", false))
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

	cmds = append(cmds, work.CmdScheduleNextJob("scheduler init"))

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
	var cmds []loop.Cmd

	switch msg := msg.(type) {
	case work.MsgJobSucceeded:
		shadowedUnits := s.Stages.MarkJobSuccess(msg.Unit)
		s.WorkerPool.Return(s.ctx, msg.Worker)

		tryMerge := s.Stages.CmdTryMerge(msg.Unit.Stage)
		if shadowedUnits == nil {
			cmds = append(cmds, tryMerge)
		} else {
			multi := []loop.Cmd{tryMerge}
			for _, u := range shadowedUnits {
				multi = append(multi, s.Stages.CmdTryMerge(u.Stage))
			}

			cmds = append(cmds,
				loop.Batch(multi...),
			)
		}

		cmds = append(cmds,
			work.CmdScheduleNextJob("job succeeded"),
		)
		if s.ExecOutWalker != nil {
			cmds = append(cmds, execout.CmdDownloadSegment(0))
		}

	case work.DelayedMsgScheduleNextJob:
		s.delayedScheduleNextJob = false

		return work.CmdScheduleNextJob("delayed:" + msg.TriggerBy)

	case work.MsgScheduleNextJob:
		s.logger.Info("scheduling next job", zap.String("trigger_by", msg.TriggerBy))
		worker, err := s.WorkerPool.Borrow(s.ctx)
		if err != nil {
			if errors.Is(err, work.ErrorResourceExhausted) {
				s.logger.Info("resource exhausted", zap.Error(err))
				if s.delayedScheduleNextJob {
					s.logger.Debug("skipping delayed schedule next job")
					return nil
				}
				s.logger.Debug("scheduling delayed schedule next job")
				s.delayedScheduleNextJob = true
				return loop.Tick(10*time.Second, work.DelayedMsgScheduleNextJob{
					TriggerBy: "resource exhausted",
				})
			} else if errors.Is(err, work.ErrorResourceExhaustedRampUp) {
				s.logger.Info("resource exhausted ramp up", zap.Error(err))

				if s.delayedScheduleNextJob {
					s.logger.Debug("skipping ramp up delayed schedule next job")
					return nil
				}
				s.logger.Debug("scheduling delayed schedule next job for ramp up")
				s.delayedScheduleNextJob = true
				return loop.Tick(1*time.Second, work.DelayedMsgScheduleNextJob{
					TriggerBy: "resource exhausted ramp up",
				})
			} else {
				s.logger.Error("scheduler: failed to borrow worker", zap.Error(err))
				return nil //todo: wrap in a retry loop or just let it go through
			}
		}
		workUnit, workRange := s.Stages.NextJob()
		if workRange == nil { // End of job
			s.WorkerPool.Return(s.ctx, worker)
			return nil
		}

		s.logger.Info("worker borrowed, scheduling work", zap.String("worker_id", worker.ID()), zap.Object("unit", workUnit))
		modules := s.Stages.StageModules(workUnit.Stage)

		return loop.Batch(
			worker.Work(s.ctx, workUnit, workRange.StartBlock, modules, s.stream),
			work.CmdScheduleNextJob("scheduler next job"),
		)

	case work.MsgJobFailed:
		s.WorkerPool.Return(s.ctx, msg.Worker)
		cmds = append(cmds, loop.Quit(msg.Error))

	case stage.MsgMergeFinished:
		s.Stages.MergeCompleted(msg.Unit)
		if !s.delayedScheduleNextJob {
			cmds = append(cmds, work.CmdScheduleNextJob("merge finished"))
		}
		cmds = append(cmds,
			s.Stages.CmdTryMerge(msg.Stage),
		)

	case stage.MsgAllStoresCompleted:
		s.storesSyncCompleted = true
		if !s.delayedScheduleNextJob {
			cmds = append(cmds, work.CmdScheduleNextJob("all store completed"))
		}
		cmds = append(cmds,
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
		} else {
			// we may be creating an index
			if s.Stages.OutputModuleIsIndex() && !s.Stages.LastStageCompleted() {
				s.logger.Info("scheduler: waiting for last stage to complete because output module is an index")
				return nil
			}
		}
		s.logger.Info("scheduler: stores and cached_outputs stream completed", fields...)
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
