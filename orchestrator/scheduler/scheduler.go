package scheduler

import (
	"context"
	"errors"
	"fmt"
	"math"
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

var errShuttingDown = errors.New("endpoint is shutting down, please reconnect")

type Scheduler struct {
	ctx context.Context
	loop.EventLoop

	stream *response.Stream

	Stages                       *stage.Stages
	WorkerPool                   work.WorkerPool
	ExecOutWalker                *execout.Walker
	StreamFirstTier2MapSegment   bool
	firstTier2MapSegmentStreamed bool

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

	if s.StreamFirstTier2MapSegment {
		cmds = append(cmds, execout.CmdWaitFirstTier2MapSegmentStreamed(250*time.Millisecond))
	} else if s.ExecOutWalker != nil {
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
	case work.MsgPendingShutdown:
		cmds = append(cmds, loop.Quit(errShuttingDown))

	case work.MsgJobSucceeded:
		shadowedUnits := s.Stages.MarkJobSuccess(msg.Unit)
		s.WorkerPool.Return(s.ctx, msg.Worker)
		if msg.Streamed {
			s.firstTier2MapSegmentStreamed = true
		}

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
		if s.ExecOutWalker != nil && (s.firstTier2MapSegmentStreamed || !s.StreamFirstTier2MapSegment) {
			cmds = append(cmds, execout.CmdDownloadSegment(0))
		}

	case work.DelayedMsgScheduleNextJob:
		s.delayedScheduleNextJob = false

		return work.CmdScheduleNextJob("delayed:" + msg.TriggerBy)

	case work.MsgScheduleNextJob:
		s.logger.Debug("scheduling next job", zap.String("trigger_by", msg.TriggerBy))

		notAboveSegment := math.MaxInt
		if s.ExecOutWalker != nil && !s.ExecOutWalker.IsNoopMode() { // noop mode is used by operators to prepare cached output, we don't want to slow down anything
			first, current, _ := s.ExecOutWalker.Progress()
			if current < first {
				current = first // cover for execoutwalker initialisation
			}
			// if we have 10 maxParallelJobs, we don't schedule jobs more than 15 segments ahead of the execout walker (1.5x)
			// This way, a client reading blocks very slowly will not cause the parallel processing of millions of blocks
			notAboveSegment = current + int(reqctx.Details(s.ctx).MaxParallelJobs)*3/2
		}

		workUnit, workRange, skippedAboveSegment := s.Stages.NextJob(notAboveSegment)
		if workRange == nil { // no job ready
			if !skippedAboveSegment {
				return nil
			}

			// this 'skippedAboveSegment' condition depends on the progress of the ExecOutWalker, which will not trigger a MsgScheduleNextJob,
			// so we need to put it back in the loop ourselves.
			// other conditions that would make new jobs available, like segment completion, will call 'MsgScheduleNextJob' themselves
			if s.delayedScheduleNextJob {
				s.logger.Debug("skipping ramp up delayed schedule next job")
				return nil
			}
			s.delayedScheduleNextJob = true
			return loop.Tick(1*time.Second, work.DelayedMsgScheduleNextJob{
				TriggerBy: "linear reader backoff",
			})
		}

		worker, err := s.WorkerPool.Borrow(s.ctx)
		if err != nil {
			s.Stages.ReleaseJob(workUnit)
			if errors.Is(err, work.ErrorResourceExhausted) {
				s.logger.Debug("resource exhausted", zap.Error(err))
				if s.delayedScheduleNextJob {
					s.logger.Debug("skipping delayed schedule next job")
					return nil
				}
				s.logger.Debug("scheduling delayed schedule next job")
				s.delayedScheduleNextJob = true
				return loop.Tick(3*time.Second, work.DelayedMsgScheduleNextJob{
					TriggerBy: "resource exhausted",
				})
			} else if errors.Is(err, work.ErrorResourceExhaustedRampUp) {
				s.logger.Debug("resource exhausted ramp up", zap.Error(err))

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

		s.logger.Debug("worker borrowed, scheduling work", zap.String("worker_id", worker.ID()), zap.Object("unit", workUnit))
		modules := s.Stages.StageModules(workUnit.Stage)

		streamOutput := s.Stages.IsFirstMapperJob(workUnit.Segment, workUnit.Stage)

		return loop.Batch(
			worker.Work(s.ctx, workUnit, workRange.StartBlock, modules, s.stream, streamOutput),
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

	case execout.MsgWaitFirstTier2MapSegmentStreamed:
		if s.firstTier2MapSegmentStreamed {
			return execout.CmdDownloadSegment(0)
		}

		return execout.CmdWaitFirstTier2MapSegmentStreamed(250 * time.Millisecond)

	case execout.MsgDownloadSegment:
		if s.ExecOutWalker == nil {
			return nil
		}
		if s.ExecOutWalker.IsWorking() {
			s.logger.Debug("scheduler: execout walker already working, skipping download request")
			return nil
		}
		s.ExecOutWalker.MarkWorking()
		if s.ExecOutWalker.IsCompleted() {
			s.logger.Debug("scheduler: execout walker completed")
			return execout.CmdWalkerCompleted()
		}
		first, current, end := s.ExecOutWalker.Progress()
		s.logger.Debug("scheduler: downloading execout segment",
			zap.Int("segment", current),
			zap.Int("first_segment", first),
			zap.Int("last_segment", end),
		)
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
			fields = append(fields,
				zap.Int("cached_output_start", start),
				zap.Int("cached_output_current", current),
				zap.Int("cached_output_end", end),
				zap.Bool("walker_is_working", s.ExecOutWalker.IsWorking()),
			)
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
