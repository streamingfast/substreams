package metrics

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The periodic progress log answers one question for whoever reads it: "why is my
// substreams slow?". It is emitted once shortly after the request starts (so a request
// that dies young still leaves a trace) and then at a slow interval, on tier1 only.
var (
	// FirstProgressLogDelay is how long we wait before the very first progress log. Short
	// enough to be useful on a request that fails early, long enough that the numbers mean
	// something. Overridable with SUBSTREAMS_PROGRESS_LOG_FIRST_DELAY.
	FirstProgressLogDelay = 1 * time.Minute
	// ProgressLogInterval is the steady-state interval between progress logs. Overridable
	// with SUBSTREAMS_PROGRESS_LOG_INTERVAL.
	ProgressLogInterval = 5 * time.Minute
)

const (
	EnvProgressLogFirstDelay = "SUBSTREAMS_PROGRESS_LOG_FIRST_DELAY"
	EnvProgressLogInterval   = "SUBSTREAMS_PROGRESS_LOG_INTERVAL"
)

func init() {
	FirstProgressLogDelay = progressLogDurationFromEnv(EnvProgressLogFirstDelay, FirstProgressLogDelay)
	ProgressLogInterval = progressLogDurationFromEnv(EnvProgressLogInterval, ProgressLogInterval)
}

// progressLogDurationFromEnv reads a duration override. A non-positive value would turn the
// reporting loop into a busy loop, so it is rejected outright rather than silently ignored:
// an operator setting this wants a specific cadence and needs to hear about a typo.
func progressLogDurationFromEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		panic(fmt.Errorf("invalid value for env var %s: %w", name, err))
	}
	if parsed <= 0 {
		panic(fmt.Errorf("invalid value for env var %s: must be greater than 0, got %s", name, value))
	}
	return parsed
}

// Bounds on what a single log line may contain. A request can have hundreds of modules and
// jobs; the line must stay readable and must never grow with the size of the run.
const (
	maxLoggedModules       = 20
	maxLoggedStages        = 12
	maxLoggedExternalCalls = 5
	maxLoggedHints         = 6
)

// Thresholds used to decide whether a hint is worth printing. They are deliberately
// generous: a hint that fires on a healthy request is worse than no hint at all.
const (
	// A tier2 job is expected to complete within 1 to 10 minutes.
	slowJobThreshold = 15 * time.Minute
	// How long a job must have been running before the rate it holds is worth extrapolating
	// from: a projection off the first couple of blocks would be noise.
	minJobRateSample = 30 * time.Second
	// Share of the window that must have been spent blocked inside SendMsg before the consumer
	// can be called the bottleneck. Set below half because the gRPC send buffer absorbs a good
	// part of a slow consumer's lag: a client taking seconds per block still leaves the server
	// unblocked most of the time, so a majority share only shows up on an extremely slow one.
	sendBlockedShareToReport = 0.35
	// A share computed over a window this short is noise, whatever it says: a request seconds
	// old has not lived long enough for any proportion to mean anything.
	minWindowForShare = 30 * time.Second
	// A single external call (eth_call & friends) taking this long is worth mentioning.
	slowExternalCallAvg = 100 * time.Millisecond
	// Calls per block above this means the module itself is call-hungry.
	highExternalCallsPerBlock = 20
	// An external call that has been waiting this long is not slow, it is stuck.
	stuckExternalCall = 30 * time.Second
	// Partial store segments waiting to be squashed. Counted in segments rather than blocks
	// so the threshold means the same thing on a chain bundling 1000 blocks per segment and
	// on one bundling 10000. A handful of partials waiting is normal — jobs finish faster than
	// the squasher merges and it catches up — so what matters is the backlog *holding*, hence
	// the second threshold.
	squashingBehindSegments = 5
	squashingBehindFor      = 2 * time.Minute
)

// StageProgress is a point-in-time view of one stage of the parallel phase: what it executes,
// the range of work it is planned to cover for this whole request, and how far it got. It is
// pushed by the orchestrator's Stages while the parallel phase runs.
type StageProgress struct {
	Stage int

	// Stores and Mappers name the modules this stage executes; index modules are reported as
	// mappers, they behave the same from here.
	Stores  []string
	Mappers []string

	// PlannedFirstJobStartBlock and PlannedLastJobStopBlock bound the work this stage is
	// expected to do over the session, straight from the request plan — not from what the
	// scheduler has gotten around to so far. Both are 0 when the stage has nothing to do.
	PlannedFirstJobStartBlock uint64
	PlannedLastJobStopBlock   uint64

	// HighestContiguousBlock is the exclusive end block up to which the whole stage is
	// immediately usable: the lowest such block across its modules, since a stage is only as
	// advanced as its least advanced module. For stores it stops at the last *squashed*
	// segment — partials that exist but were not merged yet are excluded on purpose and
	// reported separately in BlocksReadyForSquashing.
	HighestContiguousBlock uint64

	// SegmentsReadyForSquashing counts the partial store segments sitting above the
	// contiguous prefix, waiting for the squasher. Always 0 for a stage that has no store.
	SegmentsReadyForSquashing uint64
}

// stageJobStats is the per-stage job accounting behind the "jobs" section of the progress
// log. Fields prefixed with `window` are reset on every report, so they read as
// "since the last progress log" rather than "since the beginning of time".
type stageJobStats struct {
	scheduled uint64
	completed uint64
	failed    uint64
	cancelled uint64
	retried   uint64
	delayed   uint64

	lastCompletedStopBlock uint64

	window windowedStage
}

type durationStats struct {
	count   uint64
	total   time.Duration
	minimum time.Duration
	maximum time.Duration

	// blocks is the number of blocks carried by the timed messages; a message can hold
	// several blocks when the client supports buffering.
	blocks uint64
}

func (d durationStats) average() time.Duration {
	if d.count == 0 {
		return 0
	}
	return d.total / time.Duration(d.count)
}

func (d durationStats) averagePerBlock() time.Duration {
	if d.blocks == 0 {
		return 0
	}
	return d.total / time.Duration(d.blocks)
}

// MarshalLogObject reports what sending cost over the window.
//
// The timings are `SendMsg` durations, and `SendMsg` returns as soon as the message fits in
// the gRPC flow-control window — it blocks only once that window is full, until the consumer
// drains it. So the distribution is bimodal: near-zero while there is room, then one call
// stalling for as long as the consumer takes to read a whole window's worth of blocks. That
// makes a single stall a poor measure of how slow the consumer is (it measures the buffer as
// much as the client), while the total time blocked over the window is a sound one.
func (d durationStats) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddUint64("blocks", d.blocks)
	if d.count == 0 {
		// Nothing was sent over the window. Printing a wall of "0s" next to the lifetime
		// counters reads as a contradiction ("118 blocks sent, 0s to send them"), when it
		// really means the stream has not moved for the whole period.
		return nil
	}
	encoder.AddString("blocked", humanDuration(d.total))
	encoder.AddString("avg_per_block", humanDuration(d.averagePerBlock()))
	encoder.AddString("longest_stall", humanDuration(d.maximum))
	return nil
}

// RecordBlockSent should be called once per message actually carrying block data to the
// consumer, with the time the `SendMsg` call took and the number of blocks in that message
// (messages can be batched). Only the send itself must be timed: this is what tells a slow
// client apart from a slow pipeline.
func (s *Stats) RecordBlockSent(elapsed time.Duration, blockCount int) {
	s.Lock()
	defer s.Unlock()
	blocks := uint64(max(blockCount, 0))
	s.blockSendWindow.record(time.Now(), elapsed, blocks)
	s.blocksSent += blocks
}

// RecordStagesProgress is called by the orchestrator's Stages, at most once per second,
// with the per-stage, per-module state of the parallel processing.
func (s *Stats) RecordStagesProgress(progress []StageProgress) {
	s.Lock()
	defer s.Unlock()

	// Remember when each stage's squash backlog started, so a burst that the squasher works off
	// in seconds can be told apart from one it is not keeping up with.
	now := time.Now()
	if s.squashBacklogSince == nil {
		s.squashBacklogSince = make(map[int]time.Time, len(progress))
	}
	for _, stage := range progress {
		if stage.SegmentsReadyForSquashing < squashingBehindSegments {
			delete(s.squashBacklogSince, stage.Stage)
			continue
		}
		if s.squashBacklogSince[stage.Stage].IsZero() {
			s.squashBacklogSince[stage.Stage] = now
		}
	}

	s.stagesProgress = progress
}

// RecordStreamingFirstSegment flags the window during which the output the client receives
// comes straight from a tier2 job rather than from the exec-out cache. In production mode the
// first mapper segment is usually not cached yet, so tier1 has a worker stream it back live
// while the rest is being backprocessed.
func (s *Stats) RecordStreamingFirstSegment(streaming bool) {
	s.Lock()
	defer s.Unlock()
	s.streamingFirstSegment = streaming
}

// RecordJobSchedulingBlocked flags whether the scheduler is currently holding back jobs
// because they would run too far ahead of what the client has consumed. This is a normal
// back-pressure mechanism, but it is the difference between "we are slow" and "you are slow".
func (s *Stats) RecordJobSchedulingBlocked(blocked bool) {
	s.Lock()
	defer s.Unlock()
	if blocked == s.schedulingBlockedOnConsumption {
		return
	}

	now := time.Now()
	s.schedulingBlockedOnConsumption = blocked
	if blocked {
		s.schedulingBlockedSince = now
		return
	}
	// Closing an interval: only the accumulated time says whether the throttle was a blip
	// between two scheduling attempts or a state the request actually sat in.
	s.windowThrottled.add(now, now.Sub(s.schedulingBlockedSince))
	s.schedulingBlockedSince = time.Time{}
}

// RecordMaxParallelJobs records how many jobs this request may run at once.
func (s *Stats) RecordMaxParallelJobs(count uint64) {
	s.Lock()
	defer s.Unlock()
	s.maxParallelJobs = count
}

// throttledOverWindow is how long scheduling was held back over the window, including the
// interval still open.
//
// Being throttled is the normal steady state of a healthy request, not a problem: the first
// stage has no dependencies, so it races ahead until it hits the limit and sits there. The
// limit exists so that a request whose output is not advancing — because a job broke, or
// because the consumer stopped reading — does not burn workers on segments nobody may ever
// read, keeping them available for other sessions. So this number explains why few jobs are
// running; it says nothing about who is responsible for it.
//
// throttledOverWindow should be called while locked
func (s *Stats) throttledOverWindow(now time.Time, measured time.Duration) time.Duration {
	throttled := s.windowThrottled.sum(now)
	if s.schedulingBlockedOnConsumption {
		throttled += now.Sub(s.schedulingBlockedSince)
	}
	return min(throttled, measured)
}

// ProgressLogger emits the periodic "substreams request progress" log for a tier1 request.
// Emitting is decoupled from measuring: the window each `_5m` value covers is a property of
// the data, not of how often the line happens to be printed, so two consecutive lines are
// always comparable even if the interval is changed.
type ProgressLogger struct {
	stats  *Stats
	logger *zap.Logger

	firstDelay time.Duration
	interval   time.Duration
}

// NewProgressLogger reports on `stats` through `logger`, which is expected to be the request's
// own logger: the logging middleware already binds `trace_id` to it, so the line must not add
// one of its own or every entry carries the field twice.
func NewProgressLogger(stats *Stats, logger *zap.Logger) *ProgressLogger {
	return &ProgressLogger{
		stats:      stats,
		logger:     logger,
		firstDelay: FirstProgressLogDelay,
		interval:   ProgressLogInterval,
	}
}

// Run blocks until ctx is done, logging progress at the configured intervals. It also drives
// the sampling of external call totals, which has to happen on the window's own cadence: those
// totals arrive from tier2 as running sums, so a delta needs a reference point taken a window
// ago rather than at the previous log line.
func (p *ProgressLogger) Run(ctx context.Context) {
	sampler := time.NewTicker(windowBucketDuration)
	defer sampler.Stop()

	report := time.NewTimer(p.firstDelay)
	defer report.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sampler.C:
			p.stats.sampleExternalCalls(time.Now())
		case <-report.C:
			p.logProgress()
			report.Reset(p.interval)
		}
	}
}

func (p *ProgressLogger) logProgress() {
	p.logger.Info("substreams request progress", p.stats.progressFields()...)
}

// progressFields builds the whole log line under a single lock, and returns the external
// progressFields builds the whole log line under a single lock.
func (s *Stats) progressFields() []zap.Field {
	s.Lock()
	defer s.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.startTime)
	// The linear pipeline only ever runs once parallel processing handed off, so any block
	// going through it means the parallel phase is behind us.
	linearPhase := s.initDuration != 0 || s.lastProcessedBlockNum != 0

	// The phase says where the blocks the client is receiving right now come from.
	phase := "parallel_processing"
	switch {
	case linearPhase:
		phase = "linear_processing"
	case s.streamingFirstSegment:
		phase = "streaming_first_segment"
	}

	// Values suffixed `_5m` cover the trailing ProgressWindow. Until the request is that old
	// they cover its whole lifetime, which `elapsed` makes obvious.
	measured := min(elapsed, ProgressWindow)

	sendStats := s.blockSendWindow.snapshot(now)
	linearBlocksInWindow := s.windowLocalBlocks.sum(now)
	stages := s.stageJobReport(now)
	calls := s.externalCallReport(now, measured)

	// How much output is sitting in the cache that the consumer has not taken yet. Measured
	// from where the consumer actually is, which before the first block is the start of the
	// stream: measuring from 0 would report the whole chain height as "ready and waiting".
	// lastBlockInCache is where the cached output stops; the gap with last_sent_block is what
	// the consumer has left to take. Only meaningful while the output still comes from the
	// cache, the linear pipeline produces blocks as it sends them.
	var lastBlockInCache uint64
	var cachedAhead uint64
	if !linearPhase {
		lastBlockInCache = highestContiguousFor(stages, s.config.OutputModule)
		if consumedUpTo := s.consumedUpTo(); lastBlockInCache > consumedUpTo {
			cachedAhead = lastBlockInCache - consumedUpTo
		}
	}

	fields := []zap.Field{
		zap.String("user_id", s.config.UserID),
		zap.String("api_key_id", s.config.ApiKeyID),
		zap.String("output_module", s.config.OutputModule),
		zap.Bool("production_mode", s.config.ProductionMode),
		zap.String("phase", phase),
		zap.String("elapsed", humanDuration(elapsed)),
		zap.Uint64("last_sent_block", s.lastSentBlockNum),
		zap.Uint64("last_block_in_cache", lastBlockInCache),
		zap.Float64("linear_blocks_processed_per_sec_5m", perSecond(linearBlocksInWindow, measured)),
		zap.Uint64("blocks_sent", s.blocksSent),
		zap.Object("blocks_sent_5m", sendStats),
		zap.Objects("stages", stages),
		zap.Objects("external_calls", calls),
	}

	// Reported as context, not as a symptom: see throttledOverWindow.
	if throttled := s.throttledOverWindow(now, measured); throttled > 0 {
		fields = append(fields, zap.String("jobs_throttled_5m", humanDuration(throttled)))
	}

	jobErrorsInWindow := s.windowJobErrors.sum(now)
	if s.lastJobError != "" {
		fields = append(fields, zap.Object("last_job_error", &jobErrorReport{
			stage:          s.lastJobErrorStage,
			age:            now.Sub(s.lastJobErrorTime),
			total:          s.jobErrors,
			inWindow:       jobErrorsInWindow,
			message:        s.lastJobError,
			likelyExternal: looksLikeExternalCallFailure(s.lastJobError),
		}))
	}

	fields = append(fields, zap.Strings("hints", s.progressHints(stages, calls, sendStats, cachedAhead, linearPhase, measured, jobErrorsInWindow, now)))

	return fields
}

// stageJobReport summarizes, per stage, which modules it computes, how far they are, and how
// its jobs behaved over the trailing window.
//
// stageJobReport should be called while locked
func (s *Stats) stageJobReport(now time.Time) []*stageJobReport {
	running := make(map[int]*runningJobsSummary)
	for _, job := range s.runningJobs {
		summary, ok := running[int(job.Stage)]
		if !ok {
			summary = &runningJobsSummary{oldestStartBlock: job.StartBlock}
			running[int(job.Stage)] = summary
		}
		summary.count++
		age := now.Sub(job.start)
		if age > summary.oldestAge {
			summary.oldestAge = age
			summary.oldestStartBlock = job.StartBlock
			summary.oldestCurrentBlock = job.StartBlock + job.ProgressBlocks
		}

		// A job that has been running a minute and covered 33 of its 1000 blocks already tells
		// us the whole segment needs half an hour; waiting for it to actually take that long
		// before saying so wastes the half hour.
		segment := job.StopBlock - job.StartBlock
		if age < minJobRateSample || job.ProgressBlocks == 0 || segment == 0 {
			continue
		}
		projected := time.Duration(float64(age) * float64(segment) / float64(job.ProgressBlocks))
		if projected > summary.worstProjection {
			summary.worstProjection = projected
			summary.worstProjectionJob = projectedJob{
				startBlock:   job.StartBlock,
				stopBlock:    job.StopBlock,
				currentBlock: job.StartBlock + job.ProgressBlocks,
				blocks:       job.ProgressBlocks,
				age:          age,
			}
		}
	}

	planned := make(map[int]StageProgress, len(s.stagesProgress))
	for _, stage := range s.stagesProgress {
		planned[stage.Stage] = stage
	}

	count := max(len(s.stageJobs), len(s.stages), len(s.stagesProgress))

	out := make([]*stageJobReport, 0, count)
	for idx := 0; idx < count && idx < maxLoggedStages; idx++ {
		report := &stageJobReport{stage: idx}
		if stage, ok := planned[idx]; ok {
			report.stores = stage.Stores
			report.mappers = stage.Mappers
			report.plannedFirstJobStartBlock = stage.PlannedFirstJobStartBlock
			report.plannedLastJobStopBlock = stage.PlannedLastJobStopBlock
			report.segmentsReadyForSquashing = stage.SegmentsReadyForSquashing
			if since := s.squashBacklogSince[stage.Stage]; !since.IsZero() {
				report.squashBacklogFor = now.Sub(since)
			}

			// Once blocks flow through the linear pipeline, that block is the truth for every
			// module, whatever the cached files said.
			report.readyUpTo = max(stage.HighestContiguousBlock, s.lastProcessedBlockNum)
		}
		if idx < len(s.stageJobs) {
			stg := s.stageJobs[idx]
			report.scheduled = stg.scheduled
			report.completed = stg.completed
			report.failed = stg.failed
			report.cancelled = stg.cancelled
			report.retried = stg.retried
			report.delayed = stg.delayed
			report.lastCompletedStopBlock = stg.lastCompletedStopBlock

			window := stg.window.sum(now)
			report.windowCompleted = window.completed
			report.windowFailed = window.failed
			report.windowCancelled = window.cancelled
			report.windowRetried = window.retried
			report.windowDelayed = window.delayed
			report.windowMaxDuration = window.maxDuration
			if window.completed != 0 {
				report.windowAvgDuration = window.duration / time.Duration(window.completed)
			}
		}
		if summary, ok := running[idx]; ok {
			report.running = summary.count
			report.oldestRunningAge = summary.oldestAge
			report.oldestRunningStartBlock = summary.oldestStartBlock
			report.oldestRunningCurrentBlock = summary.oldestCurrentBlock
			report.worstProjection = summary.worstProjection
			report.worstProjectionJob = summary.worstProjectionJob
		}
		out = append(out, report)
	}
	return out
}

type runningJobsSummary struct {
	count     int
	oldestAge time.Duration
	// oldestStartBlock is where that job's segment begins, oldestCurrentBlock how far into it
	// the job actually got — the gap is what it has processed so far.
	oldestStartBlock   uint64
	oldestCurrentBlock uint64

	// worstProjection is how long the slowest running job's segment is on track to take at the
	// rate it has held so far.
	worstProjection    time.Duration
	worstProjectionJob projectedJob
}

// projectedJob is the evidence behind a projection: what it has covered, and in how long.
type projectedJob struct {
	startBlock   uint64
	stopBlock    uint64
	currentBlock uint64
	blocks       uint64
	age          time.Duration
}

type stageJobReport struct {
	stage   int
	stores  []string
	mappers []string

	readyUpTo                 uint64
	segmentsReadyForSquashing uint64
	squashBacklogFor          time.Duration

	scheduled uint64
	completed uint64
	failed    uint64
	cancelled uint64
	retried   uint64
	delayed   uint64

	plannedFirstJobStartBlock uint64
	plannedLastJobStopBlock   uint64
	lastCompletedStopBlock    uint64

	running                   int
	oldestRunningAge          time.Duration
	oldestRunningStartBlock   uint64
	oldestRunningCurrentBlock uint64
	worstProjection           time.Duration
	worstProjectionJob        projectedJob

	windowCompleted   uint64
	windowFailed      uint64
	windowCancelled   uint64
	windowRetried     uint64
	windowDelayed     uint64
	windowAvgDuration time.Duration
	windowMaxDuration time.Duration
}

func (j *stageJobReport) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if len(j.stores) != 0 {
		if err := encoder.AddArray("stores", stringArray(j.stores)); err != nil {
			return err
		}
	}
	if len(j.mappers) != 0 {
		if err := encoder.AddArray("mappers", stringArray(j.mappers)); err != nil {
			return err
		}
	}
	if j.readyUpTo != 0 {
		encoder.AddUint64("ready_up_to", j.readyUpTo)
	}
	if j.segmentsReadyForSquashing != 0 {
		encoder.AddUint64("squash_wait_segments", j.segmentsReadyForSquashing)
	}
	return encoder.AddObject("jobs", (*stageJobsSummary)(j))
}

// stageJobsSummary is the compact job block of a stage: the range it has to cover, and where
// it stands within it.
type stageJobsSummary stageJobReport

func (j *stageJobsSummary) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	// The bounds come from the request plan, so they are the whole range this stage has to
	// cover in this session, whether or not the scheduler got to it yet.
	if j.plannedLastJobStopBlock != 0 {
		encoder.AddUint64("start", j.plannedFirstJobStartBlock)
		encoder.AddUint64("end", j.plannedLastJobStopBlock)
	}
	encoder.AddUint64("completed", j.completed)
	encoder.AddUint64("completed_5m", j.windowCompleted)
	encoder.AddInt("running", j.running)
	if j.running != 0 {
		encoder.AddUint64("oldest_running", j.oldestRunningStartBlock)
		encoder.AddUint64("oldest_running_at", j.oldestRunningCurrentBlock)
		encoder.AddString("oldest_running_age", humanDuration(j.oldestRunningAge))
	}
	if j.windowCompleted != 0 {
		encoder.AddString("avg_dur", humanDuration(j.windowAvgDuration))
	}
	// Anything below is absent on a healthy stage: a run of zeroes on every stage of every
	// line would bury the one stage that is actually losing work.
	if j.failed != 0 || j.windowFailed != 0 {
		encoder.AddUint64("failed_total", j.failed)
		encoder.AddUint64("failed_5m", j.windowFailed)
	}
	if j.cancelled != 0 || j.windowCancelled != 0 {
		encoder.AddUint64("cancelled_total", j.cancelled)
		encoder.AddUint64("cancelled_5m", j.windowCancelled)
	}
	if j.retried != 0 || j.windowRetried != 0 {
		encoder.AddUint64("retried_total", j.retried)
		encoder.AddUint64("retried_5m", j.windowRetried)
	}
	if j.delayed != 0 || j.windowDelayed != 0 {
		encoder.AddUint64("delayed_total", j.delayed)
		encoder.AddUint64("delayed_5m", j.windowDelayed)
	}
	return nil
}

// stringArray renders a plain list of names.
type stringArray []string

func (a stringArray) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	for _, value := range a {
		encoder.AppendString(value)
	}
	return nil
}

// jobErrorReport carries the most recent tier2 job error. Only one is kept per request: when
// jobs fail in a burst they almost always share a root cause, and one readable error beats a
// list of truncated ones.
type jobErrorReport struct {
	stage    int
	age      time.Duration
	total    uint64
	inWindow uint64
	message  string
	// likelyExternal marks an error whose innermost cause points at a chain RPC endpoint
	// rather than at the module or at substreams itself.
	likelyExternal bool
}

func (e *jobErrorReport) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddInt("stage", e.stage)
	encoder.AddString("age", humanDuration(e.age))
	encoder.AddUint64("count_total", e.total)
	encoder.AddUint64("count_5m", e.inWindow)
	encoder.AddString("error", e.message)
	return nil
}

// externalCallFailureMarkers are the substrings a chain RPC failure leaves in a worker error
// once it has bubbled up through the wasm extension. Matching on text is crude, but the tier2
// protocol reports external calls as counts and durations only — the reason a call failed
// exists nowhere else by the time tier1 sees it.
var externalCallFailureMarkers = []string{
	"connection refused",
	"no such host",
	"dial tcp",
	"rpc provider",
	"json_rpc",
	"eth_call",
}

func looksLikeExternalCallFailure(message string) bool {
	lowered := strings.ToLower(message)
	for _, marker := range externalCallFailureMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// externalCallReport reports external calls (eth_call & friends) per module, both in
// absolute terms and, more usefully, over the trailing window. Lifetime totals stop telling
// you anything once a request has been running for hours.
//
// externalCallReport should be called while locked
func (s *Stats) externalCallReport(now time.Time, measured time.Duration) []*externalCallReport {
	byModule := s.wasmExtensionCallMetricsByModule()
	previous := s.windowExternalCalls.baseline(now)

	// A call made inside a tier2 job reports no block of its own, but the job it blocks does
	// not advance while the call is pending, so the job's current block is where it is stuck.
	stuckBlocks := make(map[string]uint64)
	for _, job := range s.runningJobs {
		for module := range job.modulesStats {
			if current := job.StartBlock + job.ProgressBlocks; current > stuckBlocks[module] {
				stuckBlocks[module] = current
			}
		}
	}

	// Blocks a module went through, so a call count can be turned into "calls per block",
	// which is the number that tells whether the module itself is call-hungry. Modules only
	// ever executed remotely have no local stats entry, hence the fallback.
	inFlightBlocks := s.runningJobs.blocksProcessed()
	fallbackBlocks := s.remoteProcessedBlockCount + inFlightBlocks + s.localProcessedBlockCount
	blocksByModule := make(map[string]uint64, len(s.modulesStats))
	for name, mod := range s.modulesStats {
		blocksByModule[name] = mod.processedBlocksInCompleteJobs + inFlightBlocks + s.localProcessedBlockCount
	}

	out := make([]*externalCallReport, 0, len(byModule))
	for _, metric := range byModule {
		key := metric.module + "|" + metric.extension

		report := &externalCallReport{
			module:         metric.module,
			extension:      metric.extension,
			count:          metric.count,
			totalTime:      metric.totalTime,
			maxTime:        metric.maxTime,
			inFlight:       metric.inFlight,
			oldestInFlight: metric.oldestInFlight,
			atBlock:        metric.oldestInFlightBlock,
			windowCount:    metric.count,
			windowTime:     metric.totalTime,
		}
		if prev, ok := previous[key]; ok {
			report.windowCount = metric.count - prev.count
			report.windowTime = metric.totalTime - prev.time
		}
		// Call-seconds spent per second of wall clock. A module doing many short calls stays
		// near 0; a value close to 1 means one call held the whole window, which is how a call
		// that never returns shows up when only the remote totals are available.
		if measured > 0 {
			report.avgConcurrentCalls = report.windowTime.Seconds() / measured.Seconds()
		}
		report.window = measured
		if report.atBlock == 0 {
			report.atBlock = stuckBlocks[metric.module]
		}
		blocks, ok := blocksByModule[metric.module]
		if !ok || blocks == 0 {
			blocks = fallbackBlocks
		}
		if blocks != 0 {
			report.callsPerBlock = float64(metric.count) / float64(blocks)
			report.callsPerBlockKnown = true
		}
		out = append(out, report)
	}

	// Keep only what matters: the calls that consumed the most time during the window.
	slices.SortFunc(out, func(a, b *externalCallReport) int {
		return cmp.Or(
			cmp.Compare(b.windowTime, a.windowTime),
			cmp.Compare(b.totalTime, a.totalTime),
		)
	})
	if len(out) > maxLoggedExternalCalls {
		out = out[:maxLoggedExternalCalls]
	}
	return out
}

// sampleExternalCalls stores the current cumulative external call totals so later reports can
// measure a delta against a reference point one window old.
func (s *Stats) sampleExternalCalls(now time.Time) {
	s.Lock()
	defer s.Unlock()

	byModule := s.wasmExtensionCallMetricsByModule()
	current := make(map[string]callCounters, len(byModule))
	for _, metric := range byModule {
		current[metric.module+"|"+metric.extension] = callCounters{count: metric.count, time: metric.totalTime}
	}
	s.windowExternalCalls.observe(now, current)
}

type externalCallReport struct {
	module         string
	extension      string
	count          uint64
	totalTime      time.Duration
	maxTime        time.Duration
	inFlight       uint64
	oldestInFlight time.Duration
	atBlock        uint64
	windowCount    uint64
	windowTime     time.Duration
	// callsPerBlock is only known once some block count is available for the module; a module
	// executed remotely whose jobs have not reported progress yet has none, and printing 0
	// there reads as "makes no call per block" when the truth is "we cannot tell yet".
	callsPerBlock      float64
	callsPerBlockKnown bool

	window             time.Duration
	avgConcurrentCalls float64
}

// callsStillRunning reports whether a call is waiting for an answer right now.
//
// Both tier1 and tier2 report their open calls, so the count is normally exact. The fallback
// covers a tier2 old enough to predate that reporting: it sends counts and totals only, and a
// count that does not move while the time does can only come from calls that are still waiting.
func (e *externalCallReport) callsStillRunning() bool {
	if e.inFlight != 0 {
		return true
	}
	// Close to a full window of call time with nothing having started means at least one call
	// was waiting for the entire window.
	return e.window > 0 && e.windowCount == 0 && e.avgConcurrentCalls >= 0.9
}

func (e *externalCallReport) windowAverage() time.Duration {
	if e.windowCount == 0 {
		return 0
	}
	return e.windowTime / time.Duration(e.windowCount)
}

func (e *externalCallReport) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("module", e.module)
	encoder.AddString("extension", e.extension)
	encoder.AddUint64("count_total", e.count)
	encoder.AddString("total_time", humanDuration(e.totalTime))
	encoder.AddUint64("count_5m", e.windowCount)
	encoder.AddString("time_5m", humanDuration(e.windowTime))
	encoder.AddString("avg_5m", humanDuration(e.windowAverage()))
	if e.maxTime != 0 {
		// Only calls made by this process report a per-call max; tier2 jobs send back
		// counts and totals only, and log their own max on their side.
		encoder.AddString("slowest_local_call", humanDuration(e.maxTime))
	}
	if e.inFlight != 0 {
		// Exact, but only for modules this process executed itself.
		encoder.AddUint64("in_flight", e.inFlight)
		encoder.AddString("oldest_in_flight", humanDuration(e.oldestInFlight))
	}
	if e.callsStillRunning() {
		encoder.AddBool("calls_still_running", true)
		if e.atBlock != 0 {
			encoder.AddUint64("at_block", e.atBlock)
		}
	}
	if e.callsPerBlockKnown {
		encoder.AddFloat64("calls_per_block", roundTo(e.callsPerBlock, 2))
	}
	return nil
}

// progressHints turns the numbers above into the handful of sentences a human actually
// wants: which of the usual suspects is responsible for this request being slow.
//
// progressHints should be called while locked
func (s *Stats) progressHints(
	stages []*stageJobReport,
	calls []*externalCallReport,
	sendStats durationStats,
	cachedAhead uint64,
	linearPhase bool,
	measured time.Duration,
	jobErrorsInWindow uint64,
	now time.Time,
) []string {
	var hints []string

	// 0. The stream was flowing and stopped. Everything below reads as "slow"; this one
	// says "stopped", which is a different problem and worth stating first.
	if sendStats.count == 0 && s.blocksSent != 0 {
		hints = append(hints, fmt.Sprintf(
			"no block was sent to the consumer during the last %s (the stream is stopped at block %d, %s blocks sent in total): look at the stages below to tell whether the pipeline stopped producing or the consumer stopped reading",
			humanDuration(measured), s.lastSentBlockNum, humanize.Comma(int64(s.blocksSent))))
	}

	// 1a. An external call that has not returned yet. This is the one that used to be
	// invisible: a call retrying against a dead endpoint reports no time and no failure until
	// it finally gives up, minutes later, taking the whole segment down with it.
	for _, call := range calls {
		if !call.callsStillRunning() {
			continue
		}
		// The open-call count is exact whenever the worker reports it; against a tier2 that
		// predates that reporting only the accrued time is available, so the second wording
		// says what was actually measured rather than claiming a count it does not have.
		var atBlock string
		if call.atBlock != 0 {
			atBlock = fmt.Sprintf(" on block %d", call.atBlock)
		}
		if call.inFlight != 0 && call.oldestInFlight >= stuckExternalCall {
			hints = append(hints, fmt.Sprintf(
				"module %q has %d %s call(s) still waiting for an answer%s, the oldest for %s: the endpoint behind that call is unreachable or far too slow, and the segment will eventually time out on it",
				call.module, call.inFlight, call.extension, atBlock, humanDuration(call.oldestInFlight)))
			break
		}
		if call.inFlight == 0 && call.window >= stuckExternalCall {
			hints = append(hints, fmt.Sprintf(
				"module %q spent %s of the last %s inside %s calls without a single one completing%s: at least one call has been waiting for the whole window, so the endpoint behind it is unreachable or far too slow and the segment will eventually time out on it",
				call.module, humanDuration(call.windowTime), humanDuration(call.window), call.extension, atBlock))
			break
		}
	}

	// 1b. External calls (eth_call and friends) are slow or too numerous.
	for _, call := range calls {
		if call.windowCount == 0 {
			continue
		}
		avg := call.windowAverage()
		slow := avg >= slowExternalCallAvg
		chatty := call.callsPerBlockKnown && call.callsPerBlock >= highExternalCallsPerBlock
		if !slow && !chatty {
			continue
		}

		// Name the one that actually fired: a module making two 50s calls and one making
		// thousands of 1ms calls are opposite problems with opposite fixes.
		var cause string
		switch {
		case slow && chatty:
			cause = fmt.Sprintf("each call takes %s and the module makes %.1f of them per block, so both the endpoint and the module's call volume are limiting throughput", humanDuration(avg), call.callsPerBlock)
		case slow:
			cause = fmt.Sprintf("each call takes %s on average, so the endpoint answering them is what limits throughput", humanDuration(avg))
		default:
			cause = fmt.Sprintf("the module makes %.1f of them per block, so its call volume is what limits throughput", call.callsPerBlock)
		}
		hints = append(hints, fmt.Sprintf("module %q spent %s in %d %s call(s) over the last %s: %s",
			call.module, humanDuration(call.windowTime), call.windowCount, call.extension, humanDuration(measured), cause))
		break
	}

	// 2. The consumer is what we are waiting on. The only direct evidence of that is the time
	// spent blocked inside SendMsg: how far the cache runs ahead of the consumer says nothing,
	// since the scheduler deliberately keeps it a fixed distance ahead and a healthy request
	// sits at that ceiling permanently.
	if measured >= minWindowForShare && sendStats.total >= time.Duration(float64(measured)*sendBlockedShareToReport) {
		ahead := ""
		if cachedAhead != 0 {
			ahead = fmt.Sprintf(", with %s blocks already processed and waiting in the cache", humanize.Comma(int64(cachedAhead)))
		}
		hints = append(hints, fmt.Sprintf(
			"%s of the last %s were spent blocked writing to the consumer (%s per block on average, longest single stall %s)%s: the client or the network is the bottleneck, not the processing",
			humanDuration(sendStats.total), humanDuration(measured),
			humanDuration(sendStats.averagePerBlock()), humanDuration(sendStats.maximum), ahead))
	}

	// 3. Tier2 jobs are slow, or keep getting cancelled/retried. Reported from the rate a job
	// is holding rather than from its age, so a segment on track to take half an hour is called
	// out in the first minute instead of fifteen minutes later.
	var slowest *stageJobReport
	for _, stage := range stages {
		if stage.running != 0 && (slowest == nil || stage.worstProjection > slowest.worstProjection) {
			slowest = stage
		}
	}
	switch {
	case slowest == nil:
	case slowest.worstProjection >= slowJobThreshold:
		job := slowest.worstProjectionJob
		hints = append(hints, fmt.Sprintf(
			"stage %d covered %d of the %d blocks of segment [%d, %d) in %s (%s per block): at that rate the segment needs about %s, where 1 to 10 minutes is expected",
			slowest.stage, job.blocks, job.stopBlock-job.startBlock, job.startBlock, job.stopBlock,
			humanDuration(job.age), humanDuration(job.age/time.Duration(job.blocks)),
			// Rounded: a projection off a partial segment does not deserve second precision.
			humanDuration(slowest.worstProjection.Round(time.Minute))))
	default:
		// No rate to extrapolate from on any stage: all we can say is that it has been a while.
		for _, stage := range stages {
			if stage.running != 0 && stage.oldestRunningAge >= slowJobThreshold {
				hints = append(hints, fmt.Sprintf(
					"stage %d has a job running for %s (started at block %d, reached block %d); jobs are expected to complete within 1 to 10 minutes, so this one is stuck or the module is very slow on that range",
					stage.stage, humanDuration(stage.oldestRunningAge), stage.oldestRunningStartBlock, stage.oldestRunningCurrentBlock))
				break
			}
		}
	}
	for _, stage := range stages {
		if unstable := stage.windowFailed + stage.windowCancelled + stage.windowRetried; unstable > 0 {
			hints = append(hints, fmt.Sprintf(
				"stage %d lost %d job(s) over the last %s (%d failed, %d cancelled, %d retried): work is being redone, which slows the whole request down",
				stage.stage, unstable, humanDuration(measured), stage.windowFailed, stage.windowCancelled, stage.windowRetried))
			break
		}
	}
	// 3b. Why the jobs died. The counts above say work is being redone; only the error says
	// whether the fix is on the chain endpoint, in the module, or on the tier2 fleet.
	if jobErrorsInWindow > 0 && s.lastJobError != "" {
		if looksLikeExternalCallFailure(s.lastJobError) {
			hints = append(hints, fmt.Sprintf(
				"%d job error(s) over the last %s and the last one points at a chain RPC endpoint, not at the substreams itself — check that the endpoint is reachable and keeping up: %s",
				jobErrorsInWindow, humanDuration(measured), s.lastJobError))
		} else {
			hints = append(hints, fmt.Sprintf(
				"%d job error(s) over the last %s, last one on stage %d: %s",
				jobErrorsInWindow, humanDuration(measured), s.lastJobErrorStage, s.lastJobError))
		}
	}

	// 4. Jobs produced the partials but squashing them into the full stores lags behind.
	for _, stage := range stages {
		if stage.segmentsReadyForSquashing >= squashingBehindSegments && stage.squashBacklogFor >= squashingBehindFor {
			hints = append(hints, fmt.Sprintf(
				"stage %d (%s) has had at least %d processed segments waiting to be merged for %s, %s right now (highest fully merged block is %d): squashing, not processing, is the bottleneck",
				stage.stage, strings.Join(stage.stores, ", "), squashingBehindSegments, humanDuration(stage.squashBacklogFor),
				humanize.Comma(int64(stage.segmentsReadyForSquashing)), stage.readyUpTo))
			break
		}
	}
	if !linearPhase {
		for _, stat := range s.modulesStats {
			if stat.merging && time.Since(stat.mergeBegin) > slowJobThreshold {
				hints = append(hints, fmt.Sprintf(
					"store %q has been merging a single segment for %s: the store is likely very large or the storage backend is slow",
					stat.Name, humanDuration(time.Since(stat.mergeBegin))))
				break
			}
		}
	}

	if len(hints) > maxLoggedHints {
		hints = hints[:maxLoggedHints]
	}
	return hints
}

// highestContiguousFor returns how far the stage owning the given module has been processed.
func highestContiguousFor(stages []*stageJobReport, name string) uint64 {
	for _, stage := range stages {
		if slices.Contains(stage.stores, name) || slices.Contains(stage.mappers, name) {
			return stage.readyUpTo
		}
	}
	return 0
}

// humanDuration keeps durations short and readable ("1m3s", "450ms") instead of zap's
// float-seconds rendering, because these lines are meant to be read by people.
func humanDuration(d time.Duration) string {
	switch {
	case d == 0:
		return "0s"
	case d < time.Millisecond:
		return d.Round(time.Microsecond).String()
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < time.Minute:
		return d.Round(10 * time.Millisecond).String()
	default:
		return d.Round(time.Second).String()
	}
}

func perSecond(count uint64, window time.Duration) float64 {
	if window <= 0 {
		return 0
	}
	return roundTo(float64(count)/window.Seconds(), 2)
}

func roundTo(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return float64(int64(v*pow+0.5)) / pow
}
