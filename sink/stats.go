package sink

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/internal/backprocessing"
	"github.com/streamingfast/substreams/internal/formatx"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Stats struct {
	*shutter.Shutter

	dataMsgRate                 *dmetrics.AvgRatePromCounter
	undoMsgRate                 *dmetrics.AvgRatePromCounter
	isLive                      *bool
	isIndexOutputProductionMode bool
	isNoop                      bool

	lastBlock bstream.BlockRef
	logger    *zap.Logger

	progressLock sync.Mutex
	session      *pbsubstreamsrpc.SessionInit
	progress     *pbsubstreamsrpc.ModulesProgress
	progressRate *backprocessing.RateWindow
}

func newStats(logger *zap.Logger) *Stats {
	return &Stats{
		Shutter: shutter.New(),

		dataMsgRate: dmetrics.MustNewAvgRateFromPromCounter(DataMessageCount, 1*time.Second, 30*time.Second, "msg"),
		undoMsgRate: dmetrics.MustNewAvgRateFromPromCounter(UndoMessageCount, 1*time.Second, 30*time.Second, "msg"),

		lastBlock:    unsetBlockRef{},
		progressRate: backprocessing.NewRateWindow(backprocessing.RateWindowDuration),

		logger: logger,
	}
}

func (s *Stats) RecordBlock(block bstream.BlockRef) {
	s.lastBlock = block
}

// RecordSession starts tracking backprocessing progress for a new session, discarding what the
// previous session reported.
func (s *Stats) RecordSession(session *pbsubstreamsrpc.SessionInit) {
	s.progressLock.Lock()
	defer s.progressLock.Unlock()

	s.session = session
	s.progress = nil
	s.progressRate.Reset()
}

func (s *Stats) RecordProgress(progress *pbsubstreamsrpc.ModulesProgress) {
	s.progressLock.Lock()
	defer s.progressLock.Unlock()

	s.progress = progress
	s.progressRate.Add(time.Now(), backprocessing.Done(progress))
}

func (s *Stats) SetLiveness(isLive *bool) {
	s.isLive = isLive
}

func (s *Stats) SetIndexOutputProductionMode() {
	s.isIndexOutputProductionMode = true
}

func (s *Stats) SetNoop() {
	s.isNoop = true
}

func (s *Stats) Start(each time.Duration) {
	if s.IsTerminating() || s.IsTerminated() {
		panic("already shutdown, refusing to start again")
	}

	go func() {

		if shortDelay := 2 * time.Second; shortDelay < each {
			time.Sleep(shortDelay) // first log message comes faster
			s.LogNow()
		}

		ticker := time.NewTicker(each)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.LogNow()
			case <-s.Terminating():
				return
			}
		}
	}()
}

func (s *Stats) LogNow() {

	progressLastBlock := dmetrics.NewValuesFromMetric(ProgressMessageLastBlock).Uints("stage")
	if s.isIndexOutputProductionMode || s.isNoop {
		//  'index' modules do not send any output from cached segments (in production mode), so we approximate prometheus's `HeadBlockNumber` and the logs' `last_block` from the previous `LastBlock` seen in a ProgressMessage
		var lowestStageBlock uint64
		for _, block := range progressLastBlock {
			if block < lowestStageBlock || lowestStageBlock == 0 {
				lowestStageBlock = block
			}
		}
		if s.lastBlock == nil || lowestStageBlock > s.lastBlock.Num() {
			s.lastBlock = bstream.NewBlockRef("", lowestStageBlock)
			HeadBlockNumber.SetUint64(lowestStageBlock)
		}

		// isLive is usually populated from a "handleBlockScopedData" which may not get called for 'index' modules
		if s.isLive == nil {
			falseBool := false
			s.isLive = &falseBool
		}

	}
	fields := []zap.Field{
		zap.Stringer("data_msg_rate", s.dataMsgRate),
		zap.Stringer("undo_msg_rate", s.undoMsgRate),
		zap.Float64("avg_block_wait_time_sec", BlockWaitTime.Get()),
		zap.Float64("avg_block_time_delta_sec", BlockTimeDelta.Get()),
		zap.Float64("avg_local_block_processing_time_sec", LocalProcessingTime.Get()),
		zap.Stringer("last_block", s.lastBlock),
	}
	if s.isLive == nil || !*s.isLive {
		if progress, ok := s.backprocessingProgress(); ok {
			fields = append(fields, zap.Object("backprocessing_progress", progress))
		}
	}
	if s.isLive != nil {
		fields = append(fields, zap.Bool("is_live", *s.isLive))
	}

	s.logger.Info("substreams stream stats", fields...)
}

func (s *Stats) Close() {
	s.dataMsgRate.SyncNow()
	s.undoMsgRate.SyncNow()
	s.LogNow()

	s.Shutdown(nil)
	s.dataMsgRate.Stop()
	s.undoMsgRate.Stop()
}

func (s *Stats) backprocessingProgress() (backprocessingProgress, bool) {
	s.progressLock.Lock()
	defer s.progressLock.Unlock()

	// Nothing left to backprocess: the object would only restate the same totals until the
	// stream ends, which is forever for sinks that never become live.
	if total := backprocessing.Total(s.session); total == 0 || backprocessing.Done(s.progress) >= total {
		return backprocessingProgress{}, false
	}

	rate, span, hasRate := s.progressRate.PerSecond()
	return backprocessingProgress{
		session:  s.session,
		progress: s.progress,
		rate:     rate,
		span:     span,
		hasRate:  hasRate,
	}, true
}

// backprocessingProgress logs the same figures as the `substreams run` progress view.
type backprocessingProgress struct {
	session  *pbsubstreamsrpc.SessionInit
	progress *pbsubstreamsrpc.ModulesProgress
	rate     float64
	span     time.Duration
	hasRate  bool
}

func (p backprocessingProgress) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	total := backprocessing.Total(p.session)
	done := backprocessing.Done(p.progress)
	enc.AddFloat64("pct", roundPct(100*float64(done)/float64(total)))

	if p.hasRate {
		enc.AddString("rate", formatx.Rate(p.rate, "blk"))
		if eta, ok := backprocessing.ETA(total-done, p.rate, p.span); ok {
			enc.AddString("eta", formatx.Duration(eta))
		}
	}

	var jobs int
	if p.progress != nil {
		jobs = len(p.progress.RunningJobs)
	}
	if p.session.MaxParallelWorkers != 0 {
		enc.AddString("jobs", fmt.Sprintf("%d/%d", jobs, p.session.MaxParallelWorkers))
	} else {
		enc.AddInt("jobs", jobs)
	}

	enc.AddUint64("blocks_to_prepare_stores", p.session.EffectiveBlocksToProcessBeforeStartBlock)
	enc.AddUint64("blocks_in_range", p.session.EffectiveBlocksToProcessAfterStartBlock)
	enc.AddUint64("blocks_cached", backprocessing.Cached(p.session))

	if p.progress != nil && len(p.progress.Stages) != 0 {
		return enc.AddObject("stages", stagesProgress(p))
	}
	return nil
}

type stagesProgress backprocessingProgress

func (p stagesProgress) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	stats := backprocessing.StageStats(p.progress.Stages, p.progress.RunningJobs, backprocessing.ParallelTarget(p.session))
	for _, stat := range stats {
		if err := enc.AddObject(fmt.Sprintf("s%d", stat.Index), stageProgress(stat)); err != nil {
			return err
		}
	}

	if lo, frontier, target, ok := backprocessing.OutFrontier(p.session, p.progress); ok {
		return enc.AddObject("out", outProgress{lo: lo, frontier: frontier, target: target})
	}
	return nil
}

type stageProgress backprocessing.StageStat

func (s stageProgress) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddFloat64("pct", roundPct(100*s.Ratio))
	enc.AddUint64("first_block", s.Low)
	if s.Ready > s.Low {
		enc.AddUint64("ready_up_to", s.Ready)
	}
	enc.AddInt("jobs", s.Jobs)
	if s.Jobs != 0 {
		enc.AddString("oldest_job", formatx.Duration(s.OldestJob))
	}
	if s.SquashWait != 0 {
		enc.AddUint64("squashing_segments", s.SquashWait)
	}
	return nil
}

type outProgress struct {
	lo, frontier, target uint64
}

func (o outProgress) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddFloat64("pct", roundPct(100*float64(o.frontier-o.lo)/float64(o.target-o.lo)))
	enc.AddUint64("first_block", o.lo)
	if o.frontier > o.lo {
		enc.AddUint64("ready_up_to", o.frontier)
	}
	enc.AddUint64("start_block", o.target)
	return nil
}

func roundPct(pct float64) float64 {
	return math.Round(pct*10) / 10
}

type unsetBlockRef struct{}

func (unsetBlockRef) ID() string     { return "" }
func (unsetBlockRef) Num() uint64    { return 0 }
func (unsetBlockRef) String() string { return "None" }
