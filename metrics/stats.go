package metrics

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dmetrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Stats struct {
	sync.Mutex

	config *Config

	blockRate *dmetrics.AvgRateCounter

	startTime       time.Time
	stages          []*pbsubstreamsrpc.Stage
	initDuration    time.Duration
	timeToFirstData time.Duration

	// modulesStats only contain stats from local execution
	modulesStats map[string]*extendedStats

	runningJobs             runningJobs
	completedJobsStats      map[string]*pbssinternal.ModuleStats
	uncompressedEgressBytes uint64
	// processedBlocks is written from the block-processing goroutine and read from tier2's
	// segment watchdog goroutine (which uses it as a liveness signal), so it must be atomic.
	processedBlocks atomic.Uint64

	localProcessedBlockCount  uint64
	remoteProcessedBlockCount uint64
	completedJobsBytesRead    uint64
	completedJobsBytesWritten uint64
	// successfully completed tier2 job (could be a noop if the tier2 finds out that all its required files are there upon startup)
	completedJobs uint64
	// jobs that failed (either with a fatal error or repeatedly on retryable errors, above a threshold)
	failedJobs uint64

	// retries that happened (does not affect the total number of jobs, a single startedJob can be retried multiple times)
	retriedJobs uint64

	// jobs that were delayed (waiting for overloaded tier2, does not affect the total number of jobs, a single startedJob can be delayed multiple times)
	delayedJobs uint64

	// tier2 jobs that were started
	startedJobs uint64

	// counter is used to get the next jobIdx
	counter uint64

	clientReadTime *dmetrics.AvgDurationCounter
	error          error
	logger         *zap.Logger
	stores         []*pbsubstreams.Module
	moduleHashes   map[string]string

	lastSentBlockNum  uint64
	lastSentBlockID   string
	lastSentBlockTime time.Time
	// resolvedStartBlockNum is where the stream starts, so a request that has not sent
	// anything yet still has a baseline to measure the consumer against. Without it, "0"
	// stands in for "nothing sent" and every distance computed from it is nonsense.
	resolvedStartBlockNum uint64

	// ---- fields feeding the periodic "substreams request progress" log (tier1) ----

	// stagesProgress is refreshed by the orchestrator's Stages while parallel processing runs.
	stagesProgress []StageProgress
	// squashBacklogSince records, per stage, when its squash backlog first crossed the
	// reporting threshold, so only a backlog that holds is reported.
	squashBacklogSince map[int]time.Time
	// lastProcessedBlockNum is the last block that went through the linear pipeline, which
	// supersedes modulesProgress once we left the parallel phase.
	lastProcessedBlockNum uint64
	// streamingFirstSegment is set while a tier2 job streams the first mapper segment straight
	// to the client, instead of tier1 reading it from the exec-out cache.
	streamingFirstSegment bool
	// schedulingBlockedOnConsumption is set when the scheduler refuses to schedule further
	// jobs because they would run too far ahead of what the client consumed.
	schedulingBlockedOnConsumption bool
	schedulingBlockedSince         time.Time
	// windowThrottled is how long scheduling was actually held back over the window. The flag
	// above is a momentary state that flips on every scheduling attempt, so it says nothing on
	// its own about whether the throttle cost anything.
	windowThrottled windowedDuration
	// maxParallelJobs is what the request is allowed to run at once, so an idle worker count
	// can be derived: a throttle only costs something when it leaves workers with nothing to do.
	maxParallelJobs uint64
	// stageJobs holds job accounting per stage, indexed by stage number.
	stageJobs []*stageJobStats
	// lastJobError keeps the most recent tier2 job error of the request. Failure counts tell
	// you that jobs are being redone; only the error text tells you why (an unreachable RPC
	// endpoint behind an eth_call, a deterministic module panic, an overloaded tier2...).
	lastJobError      string
	lastJobErrorStage int
	lastJobErrorTime  time.Time
	jobErrors         uint64
	windowJobErrors   windowedCounter
	// blockSendWindow times the individual `SendMsg` calls carrying block data, so we can tell
	// a slow consumer apart from slow processing.
	blockSendWindow windowedDurations
	blocksSent      uint64
	// windowLocalBlocks counts blocks that went through the linear pipeline, over the window.
	windowLocalBlocks windowedCounter
	// windowExternalCalls turns the cumulative external-call totals into a windowed delta.
	windowExternalCalls windowedCallCounters
}

type runningJobs map[uint64]*extendedJob

func cloneStats(in *pbssinternal.ModuleStats) *pbssinternal.ModuleStats {
	return &pbssinternal.ModuleStats{
		Name:                   in.Name,
		ProcessingTimeMs:       in.ProcessingTimeMs,
		StoreOperationTimeMs:   in.StoreOperationTimeMs,
		StoreReadCount:         in.StoreReadCount,
		ExternalCallMetrics:    cloneCallMetrics(in.ExternalCallMetrics),
		StoreWriteCount:        in.StoreWriteCount,
		StoreDeleteprefixCount: in.StoreDeleteprefixCount,
		StoreSizeBytes:         in.StoreSizeBytes,
	}
}

func (j runningJobs) blocksProcessed() (count uint64) {
	for _, job := range j {
		count += job.ProgressBlocks
	}
	return
}

func (j runningJobs) ModuleStats(module string) (out *pbssinternal.ModuleStats) {
	for _, job := range j {
		for _, stat := range job.modulesStats {
			if stat.Name == module {
				if out == nil {
					out = cloneStats(stat)
					continue
				}
				mergeModuleStats(out, stat)
			}
		}
	}
	return
}

// mergeModuleStats merges right onto left
func mergeModuleStats(left, right *pbssinternal.ModuleStats) {
	if right == nil {
		return
	}
	left.ProcessingTimeMs += right.ProcessingTimeMs
	left.StoreOperationTimeMs += right.StoreOperationTimeMs
	left.StoreReadCount += right.StoreReadCount
	left.ExternalCallMetrics = mergeCallMetricsSlices(left.ExternalCallMetrics, right.ExternalCallMetrics)
	left.StoreWriteCount += right.StoreWriteCount
	left.StoreDeleteprefixCount += right.StoreDeleteprefixCount
	if right.StoreSizeBytes > left.StoreSizeBytes {
		left.StoreSizeBytes = right.StoreSizeBytes
	}
}

// mergeMixedModuleStats merges right onto left
func mergeMixedModuleStats(left *pbsubstreamsrpc.ModuleStats, right *pbssinternal.ModuleStats) {
	if right == nil {
		return
	}
	left.TotalProcessingTimeMs += right.ProcessingTimeMs
	left.TotalStoreOperationTimeMs += right.StoreOperationTimeMs
	left.TotalStoreReadCount += right.StoreReadCount
	left.ExternalCallMetrics = mergeMixedCallMetrics(left.ExternalCallMetrics, right.ExternalCallMetrics)
	left.TotalStoreWriteCount += right.StoreWriteCount
	left.TotalStoreDeleteprefixCount += right.StoreDeleteprefixCount
	if right.StoreSizeBytes > left.StoreSizeBytes {
		left.StoreSizeBytes = right.StoreSizeBytes
	}
}

type extendedJob struct {
	*pbsubstreamsrpc.Job
	modulesStats map[string]*pbssinternal.ModuleStats
	start        time.Time
	bytesRead    uint64
	bytesWritten uint64
}

// RecordJobUpdate will be called each time a job sends an update message
func (s *Stats) RecordJobUpdate(jobIdx uint64, upd *pbssinternal.Update) {
	s.Lock()
	defer s.Unlock()

	job := s.runningJobs[jobIdx]
	for _, modStatUpdate := range upd.ModulesStats {
		job.modulesStats[modStatUpdate.Name] = modStatUpdate
	}
	job.ProgressBlocks = upd.ProgressBlocks
	job.DurationMs = upd.DurationMs
	job.bytesRead = upd.TotalBytesRead
	job.bytesWritten = upd.TotalBytesWritten
}

func NewReqStats(config *Config, stores []*pbsubstreams.Module, moduleHashes map[string]string, logger *zap.Logger) *Stats {
	return &Stats{
		config:             config,
		blockRate:          dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		clientReadTime:     dmetrics.NewAvgDurationCounter(5*time.Minute, time.Second, "client_read_time"),
		startTime:          time.Now(),
		logger:             logger,
		modulesStats:       make(map[string]*extendedStats),
		runningJobs:        make(map[uint64]*extendedJob),
		completedJobsStats: make(map[string]*pbssinternal.ModuleStats),
		stores:             stores,
		moduleHashes:       moduleHashes,
	}
}

func (s *Stats) SetError(err error) {
	s.error = err
}

type extendedStats struct {
	*pbssinternal.ModuleStats
	merging                       bool
	mergeBegin                    time.Time
	mergingTime                   time.Duration
	processedBlocksInCompleteJobs uint64
	storeOperationTime            time.Duration
	processingTime                time.Duration

	// uniqueID -> startTime
	inprocessSince map[uint64]time.Time

	// extension --> metric
	externalCallMetrics map[string]*extendedCallMetric

	// uniqueID -> metric
	inprocessCallMetrics map[uint64]inprocessCall
}

type inprocessCall struct {
	startTime time.Time
	extension string
	// blockNum is the block the module was executing when it made the call, which is where
	// processing is stuck for as long as the call does not return.
	blockNum uint64
}

type extendedCallMetric struct {
	count uint64
	// failed counts the calls that came back with an error. A chain endpoint that is refusing
	// connections shows up here long before the segment gives up on it.
	failed uint64
	time   time.Duration
	// maxTime is the duration of the slowest single call, which a total or an average hides: one
	// 30s eth_call among thousands of fast ones barely moves the average.
	maxTime time.Duration
}

// updateDurations should be called while locked
func (s *extendedStats) updateDurations() {
	s.ModuleStats.ProcessingTimeMs = uint64(s.processingTime.Milliseconds())
	for _, inproc := range s.inprocessSince {
		s.ModuleStats.ProcessingTimeMs += uint64(time.Since(inproc).Milliseconds())
	}

	s.ModuleStats.ExternalCallMetrics = make([]*pbssinternal.ExternalCallMetric, len(s.externalCallMetrics))
	i := 0
	for k, v := range s.externalCallMetrics {
		callMetric := &pbssinternal.ExternalCallMetric{
			Name:        k,
			Count:       v.count,
			TimeMs:      uint64(v.time.Milliseconds()),
			FailedCount: v.failed,
		}
		// A call that has not returned has already been counted (the count is incremented when
		// it starts) but contributed no time yet. Reported as-is, a call hung for minutes looks
		// instantaneous and a whole class of problems stays invisible until the segment dies.
		for _, inproc := range s.inprocessCallMetrics {
			if inproc.extension != k {
				continue
			}
			waiting := time.Since(inproc.startTime)
			callMetric.TimeMs += uint64(waiting.Milliseconds())
			callMetric.InFlightCount++
			if waiting.Milliseconds() > int64(callMetric.OldestInFlightMs) {
				callMetric.OldestInFlightMs = uint64(waiting.Milliseconds())
				callMetric.OldestInFlightBlock = inproc.blockNum
			}
		}

		s.ModuleStats.ExternalCallMetrics[i] = callMetric
		i++
	}

	if len(s.ModuleStats.ExternalCallMetrics) > 0 {
		slices.SortFunc(s.ModuleStats.ExternalCallMetrics, func(a, b *pbssinternal.ExternalCallMetric) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	s.ModuleStats.StoreOperationTimeMs = uint64(s.storeOperationTime.Milliseconds())
}

func (s *Stats) RecordInitializationComplete() {
	s.Lock()
	defer s.Unlock()
	s.initDuration = time.Since(s.startTime)
	// No more jobs to hold back once parallel processing is over.
	s.schedulingBlockedOnConsumption = false
	s.schedulingBlockedSince = time.Time{}
}

func (s *Stats) RecordEgress(egressBytes int) {
	// this is always sent linearly, no need to lock
	s.uncompressedEgressBytes += uint64(egressBytes)
}

func (s *Stats) RecordDataSent() {
	// this is always sent linearly, no need to lock
	if s.timeToFirstData == 0 {
		s.timeToFirstData = time.Since(s.startTime)
	}
}

// RecordLastBlockSent keeps track of the last block that was sent to the client, reported in the
// final "substreams request stats" log. Sent linearly, no need to lock.
func (s *Stats) RecordLastBlockSent(clock *pbsubstreams.Clock) {
	if clock == nil {
		return
	}
	s.lastSentBlockNum = clock.Number
	s.lastSentBlockID = clock.Id
	s.lastSentBlockTime = clock.Timestamp.AsTime()
}

// RecordResolvedStartBlock sets the block the stream starts at, once it is known.
func (s *Stats) RecordResolvedStartBlock(blockNum uint64) {
	s.Lock()
	defer s.Unlock()
	s.resolvedStartBlockNum = blockNum
}

// consumedUpTo is the highest block the consumer can be said to have gone through. Before
// the first block is sent that is the start of the stream, not block 0.
//
// consumedUpTo should be called while locked
func (s *Stats) consumedUpTo() uint64 {
	if s.lastSentBlockNum != 0 {
		return s.lastSentBlockNum
	}
	return s.resolvedStartBlockNum
}

func (s *Stats) RecordBlocksProcessed(count uint64) {
	s.processedBlocks.Add(count)
}

func (s *Stats) GetBlocksProcessed() uint64 {
	return s.processedBlocks.Load()
}

func (s *Stats) RecordStages(stages []*pbsubstreamsrpc.Stage) {
	s.Lock()
	defer s.Unlock()
	s.stages = stages
}

func (s *Stats) Stages() []*pbsubstreamsrpc.Stage {
	s.Lock()
	defer s.Unlock()
	return s.stages
}

func (s *Stats) RecordNewSubrequest(stage uint32, startBlock, stopBlock uint64) (id uint64) {
	s.Lock()
	id = s.counter
	s.counter++

	s.startedJobs++
	s.runningJobs[id] = &extendedJob{
		start: time.Now(),
		Job: &pbsubstreamsrpc.Job{
			Stage:          stage,
			StartBlock:     startBlock,
			StopBlock:      stopBlock,
			ProgressBlocks: 0,
			DurationMs:     0,
		},
		modulesStats: make(map[string]*pbssinternal.ModuleStats),
	}

	s.stageJobStats(int(stage)).scheduled++

	s.Unlock()
	return id
}

// stageJobStats should be called while locked
func (s *Stats) stageJobStats(stage int) *stageJobStats {
	for len(s.stageJobs) <= stage {
		s.stageJobs = append(s.stageJobs, &stageJobStats{})
	}
	return s.stageJobs[stage]
}

func (s *Stats) RecordModuleMerging(module string) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.modulesStats[module]; !ok {
		s.modulesStats[module] = newExtendedStats(module)
	}
	s.modulesStats[module].merging = true
	s.modulesStats[module].mergeBegin = time.Now()
}

func (s *Stats) RecordModuleMergeComplete(module string) {
	s.Lock()
	defer s.Unlock()
	stat := s.modulesStats[module]
	stat.merging = false
	stat.mergingTime += time.Since(stat.mergeBegin)
}

// JobStatus represents the final state of a job
type JobStatus int

const (
	JobComplete JobStatus = iota
	JobCancelled
	JobFailed
)

// maxLoggedJobError bounds how much of a job error reaches the progress log. Worker errors
// are deeply wrapped and can carry a payload dump; the interesting part (the innermost
// cause, e.g. "connection refused" under an eth_call) sits well past the first few hundred
// characters, so the cap has to be generous, but only one error is ever kept.
const maxLoggedJobError = 900

// RecordJobError should be called whenever a tier2 job comes back with an error, whether it
// will be retried or not.
func (s *Stats) RecordJobError(jobIdx uint64, err error) {
	// A cancellation is the request going away, not a job going wrong: reporting it would
	// bury the real error under noise every time a client disconnects.
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	s.Lock()
	defer s.Unlock()

	s.jobErrors++
	s.windowJobErrors.add(time.Now(), 1)
	s.lastJobError = truncateError(err.Error())
	s.lastJobErrorTime = time.Now()
	if job, ok := s.runningJobs[jobIdx]; ok {
		s.lastJobErrorStage = int(job.Stage)
	}
}

func truncateError(in string) string {
	if len(in) <= maxLoggedJobError {
		return in
	}
	return in[:maxLoggedJobError] + "…(truncated)"
}

// RecordJobDelayed should be called when a job is retried without any work done (ex: rejected upon connection to tier2)
func (s *Stats) RecordJobDelayed(jobIdx uint64) {
	s.Lock()
	defer s.Unlock()
	s.delayedJobs++
	if job, ok := s.runningJobs[jobIdx]; ok {
		stg := s.stageJobStats(int(job.Stage))
		stg.delayed++
		stg.window.bucket(time.Now()).delayed++
	}
}

// RecordJobRetried should be called when a job is retried after having possibly done some work
func (s *Stats) RecordJobRetried(jobIdx uint64) {
	s.Lock()
	defer s.Unlock()
	s.retriedJobs++
	if job, ok := s.runningJobs[jobIdx]; ok {
		stg := s.stageJobStats(int(job.Stage))
		stg.retried++
		stg.window.bucket(time.Now()).retried++
	}
}

func (s *Stats) RecordEndSubrequest(jobIdx uint64, status JobStatus) {
	s.Lock()
	defer s.Unlock()
	job := s.runningJobs[jobIdx]

	for i := 0; i <= int(job.Stage); i++ {
		for _, mod := range s.stages[i].Modules {
			if _, ok := s.modulesStats[mod]; !ok {
				s.modulesStats[mod] = newExtendedStats(mod)
			}
			s.modulesStats[mod].processedBlocksInCompleteJobs += job.ProgressBlocks
		}
	}

	for name, jobStats := range job.modulesStats {
		modStat, ok := s.completedJobsStats[name]
		if !ok {
			s.completedJobsStats[name] = jobStats
			continue
		}
		mergeModuleStats(modStat, jobStats)
	}
	s.completedJobsBytesRead += job.bytesRead
	s.completedJobsBytesWritten += job.bytesWritten

	stg := s.stageJobStats(int(job.Stage))
	bucket := stg.window.bucket(time.Now())
	elapsed := time.Since(job.start)
	switch status {
	case JobComplete:
		s.completedJobs++
		stg.completed++
		bucket.completed++
		bucket.duration += elapsed
		if elapsed > bucket.maxDuration {
			bucket.maxDuration = elapsed
		}
		if job.StopBlock > stg.lastCompletedStopBlock {
			stg.lastCompletedStopBlock = job.StopBlock
		}
	case JobCancelled:
		stg.cancelled++
		bucket.cancelled++
	case JobFailed:
		s.failedJobs++
		stg.failed++
		bucket.failed++
	}
	s.remoteProcessedBlockCount += job.ProgressBlocks

	delete(s.runningJobs, jobIdx)
}

// RecordModuleWasmBlockBegin should be called once per module per block
func (s *Stats) RecordModuleWasmBlockBegin(moduleName string) uint64 {
	s.Lock()
	defer s.Unlock()
	uniqueID := uniqueIDCounter.Inc()
	mod := s.moduleStats(moduleName)
	mod.inprocessSince[uniqueID] = time.Now()

	return uniqueID
}

// RecordModuleWasmBlockEnd should be called once per module per block. `elapsed` is the time spent in executing the WASM code, including store and extension calls
func (s *Stats) RecordModuleWasmBlockEnd(moduleName string, uniqueID uint64) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.processingTime += time.Since(mod.inprocessSince[uniqueID])
	delete(mod.inprocessSince, uniqueID)
}

var uniqueIDCounter = atomic.NewUint64(0)

// RecordModuleWasmExternalCallBegin can be called multiple times per module per block, for each external module call (ex: eth_call).
func (s *Stats) RecordModuleWasmExternalCallBegin(moduleName string, extension string, blockNum uint64) uint64 {
	s.Lock()
	defer s.Unlock()

	mod := s.moduleStats(moduleName)
	uniqueID := uniqueIDCounter.Inc()

	// initialize map
	mod.inprocessCallMetrics[uniqueID] = inprocessCall{
		startTime: time.Now(),
		extension: extension,
		blockNum:  blockNum,
	}

	met, ok := mod.externalCallMetrics[extension]
	if !ok {
		met = &extendedCallMetric{}
		mod.externalCallMetrics[extension] = met
	}
	met.count++

	return uniqueID
}

// RecordModuleWasmExternalCallEnd can be called multiple times per module per block, for each external module call (ex: eth_call). `elapsed` is the time spent in executing that call.
func (s *Stats) RecordModuleWasmExternalCallEnd(moduleName string, extension string, uniqueID uint64, callErr error) {
	s.Lock()
	defer s.Unlock()

	mod := s.moduleStats(moduleName)
	met, ok := mod.externalCallMetrics[extension]
	if !ok {
		met = &extendedCallMetric{}
		mod.externalCallMetrics[extension] = met
	}
	inproc := mod.inprocessCallMetrics[uniqueID]
	elapsed := time.Since(inproc.startTime)
	met.time += elapsed
	if elapsed > met.maxTime {
		met.maxTime = elapsed
	}
	if callErr != nil {
		met.failed++
	}

	delete(mod.inprocessCallMetrics, uniqueID)
}

// RecordModuleWasmStoreRead can be called multiple times per module per block `elapsed` is the time spent in executing that operation.
func (s *Stats) RecordModuleWasmStoreRead(moduleName string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.StoreReadCount++
	mod.storeOperationTime += elapsed
}

// RecordModuleWasmStoreWrite can be called multiple times per module per block `elapsed` is the time spent in executing that operation.
func (s *Stats) RecordModuleWasmStoreWrite(moduleName string, sizeBytes uint64, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.StoreSizeBytes = sizeBytes
	mod.StoreWriteCount++
	mod.storeOperationTime += elapsed
}

// RecordModuleWasmStoreDeletePrefix can be called multiple times per module per block `elapsed` is the time spent in executing that operation.
func (s *Stats) RecordModuleWasmStoreDeletePrefix(moduleName string, sizeBytes uint64, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.StoreSizeBytes = sizeBytes
	mod.StoreDeleteprefixCount++
	mod.storeOperationTime += elapsed
}

func (s *Stats) RecordReadTime(since time.Time) {
	s.Lock()
	defer s.Unlock()
	s.clientReadTime.AddElapsedTime(since)
}

func (s *Stats) RecordBlock(ref bstream.BlockRef) {
	s.Lock()
	defer s.Unlock()
	s.blockRate.Add(1)
	s.localProcessedBlockCount += 1
	s.windowLocalBlocks.add(time.Now(), 1)
	if ref != nil {
		s.lastProcessedBlockNum = ref.Num()
	}
}

func newExtendedStats(moduleName string) *extendedStats {
	return &extendedStats{
		ModuleStats: &pbssinternal.ModuleStats{
			Name: moduleName,
		},
		externalCallMetrics:  make(map[string]*extendedCallMetric),
		inprocessCallMetrics: make(map[uint64]inprocessCall),
		inprocessSince:       make(map[uint64]time.Time),
	}
}

// moduleStats should be called while locked
func (s *Stats) moduleStats(moduleName string) *extendedStats {
	mod, ok := s.modulesStats[moduleName]
	if !ok {
		mod = newExtendedStats(moduleName)
		s.modulesStats[moduleName] = mod
	}
	return mod
}

type Config struct {
	UserID           string
	ApiKeyID         string
	OutputModule     string
	OutputModuleHash string
	ProductionMode   bool
	Tier2            bool
}

func (s *Stats) JobsStats() []*pbsubstreamsrpc.Job {
	s.Lock()
	defer s.Unlock()

	out := make([]*pbsubstreamsrpc.Job, len(s.runningJobs))
	i := 0
	for _, v := range s.runningJobs {
		out[i] = &pbsubstreamsrpc.Job{
			Stage:          v.Stage,
			StartBlock:     v.StartBlock,
			StopBlock:      v.StopBlock,
			ProgressBlocks: v.ProgressBlocks,
			DurationMs:     uint64(time.Since(v.start).Milliseconds()),
		}
		i++
	}

	return out
}

func (s *Stats) LocalModulesStats() []*pbssinternal.ModuleStats {
	s.Lock()
	defer s.Unlock()

	out := make([]*pbssinternal.ModuleStats, len(s.modulesStats))
	i := 0
	for k, v := range s.modulesStats {
		v.updateDurations()
		out[i] = &pbssinternal.ModuleStats{
			Name:                   k,
			ProcessingTimeMs:       uint64(v.processingTime.Milliseconds()),
			StoreOperationTimeMs:   uint64(v.storeOperationTime.Milliseconds()),
			StoreReadCount:         v.StoreReadCount,
			ExternalCallMetrics:    v.ExternalCallMetrics,
			StoreWriteCount:        v.StoreWriteCount,
			StoreDeleteprefixCount: v.StoreDeleteprefixCount,
			StoreSizeBytes:         v.StoreSizeBytes,
		}

		i++
	}

	return out
}

func toRPCCallMetrics(in []*pbssinternal.ExternalCallMetric) (out []*pbsubstreamsrpc.ExternalCallMetric) {
	if in == nil {
		return nil
	}
	out = make([]*pbsubstreamsrpc.ExternalCallMetric, len(in))
	for i := range in {
		out[i] = &pbsubstreamsrpc.ExternalCallMetric{
			Name:   in[i].Name,
			Count:  in[i].Count,
			TimeMs: in[i].TimeMs,
		}
	}
	return
}

// modifies 'left' slice
func mergeCallMetricsSlices(left, right []*pbssinternal.ExternalCallMetric) []*pbssinternal.ExternalCallMetric {
	for _, r := range right {
		var seen bool
		for _, l := range left {
			if l.Name == r.Name {
				l.TimeMs += r.TimeMs
				l.Count += r.Count
				seen = true
			}
		}
		if !seen {
			left = append(left, r)
		}
	}

	return left
}

// modifies 'left' slice
func mergeMixedCallMetrics(left []*pbsubstreamsrpc.ExternalCallMetric, right []*pbssinternal.ExternalCallMetric) []*pbsubstreamsrpc.ExternalCallMetric {
	for _, r := range right {
		var seen bool
		for _, l := range left {
			if l.Name == r.Name {
				l.TimeMs += r.TimeMs
				l.Count += r.Count
				seen = true
			}
		}
		if !seen {
			left = append(left, &pbsubstreamsrpc.ExternalCallMetric{
				Name:   r.Name,
				Count:  r.Count,
				TimeMs: r.TimeMs,
			})
		}
	}

	return left
}

func cloneCallMetrics(in []*pbssinternal.ExternalCallMetric) []*pbssinternal.ExternalCallMetric {
	out := make([]*pbssinternal.ExternalCallMetric, len(in))
	for i := range in {
		out[i] = &pbssinternal.ExternalCallMetric{
			Name:   in[i].Name,
			Count:  in[i].Count,
			TimeMs: in[i].TimeMs,
		}
	}
	return out
}

func (s *Stats) stage(module string) (uint32, *pbsubstreamsrpc.Stage) {
	for i, ss := range s.stages {
		if slices.Contains(ss.Modules, module) {
			return uint32(i), ss
		}
	}
	// could happen on initial lookup, minor race condition
	return 0, nil
}

func (s *Stats) RemoteBytesConsumption() (read uint64, written uint64) {
	s.Lock()
	defer s.Unlock()
	return s.remoteBytesConsumption()
}

func (s *Stats) remoteBytesConsumption() (read uint64, written uint64) {
	read = s.completedJobsBytesRead
	written = s.completedJobsBytesWritten
	for _, j := range s.runningJobs {
		read += j.bytesRead
		written += j.bytesWritten
	}

	return read, written
}

func (s *Stats) AggregatedModulesStats() []*pbsubstreamsrpc.ModuleStats {
	s.Lock()
	defer s.Unlock()

	out := make([]*pbsubstreamsrpc.ModuleStats, len(s.modulesStats))
	i := 0
	for k, v := range s.modulesStats {
		v.updateDurations()
		out[i] = &pbsubstreamsrpc.ModuleStats{
			Name:                        k,
			TotalProcessingTimeMs:       uint64(v.processingTime.Milliseconds()),
			TotalStoreOperationTimeMs:   uint64(v.storeOperationTime.Milliseconds()),
			TotalStoreReadCount:         v.StoreReadCount,
			ExternalCallMetrics:         toRPCCallMetrics(v.ExternalCallMetrics),
			TotalStoreWriteCount:        v.StoreWriteCount,
			TotalStoreDeleteprefixCount: v.StoreDeleteprefixCount,
			StoreSizeBytes:              v.StoreSizeBytes,
			TotalProcessedBlockCount:    v.processedBlocksInCompleteJobs + s.runningJobs.blocksProcessed() + s.localProcessedBlockCount,
			TotalStoreMergingTimeMs:     uint64(v.mergingTime.Milliseconds()),
			StoreCurrentlyMerging:       v.merging,
		}

		mergeMixedModuleStats(out[i], s.runningJobs.ModuleStats(k))
		mergeMixedModuleStats(out[i], s.completedJobsStats[k])
		_, stage := s.stage(v.Name)
		if stage != nil { // will be nil for mappers
			if ranges := stage.CompletedRanges; ranges != nil {
				out[i].HighestContiguousBlock = ranges[0].EndBlock
			}
		}
		i++
	}

	return out
}

func (s *Stats) LogAndClose(ctx context.Context, resolvedStartBlockNum uint64) {
	s.Lock()
	defer s.Unlock()
	s.blockRate.SyncNow()
	s.blockRate.Stop()
	meter := dmetering.GetBytesMeter(ctx)
	zapFields := s.getZapFields(meter)
	zapFields = append(zapFields, zap.Uint64("resolved_start_block", resolvedStartBlockNum))

	// WARNING: DO NOT MODIFY THIS LOG ENTRY, IT IS USED IN REPORTING SYSTEMS ##################
	s.logger.Info("substreams request stats", zapFields...)
	// ####################################################################################

}

type storeSize struct {
	name string
	hash string
	size uint64
}

func (s *storeSize) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("name", s.name)
	encoder.AddString("hash", s.hash)
	encoder.AddUint64("size", s.size)
	return nil
}

// getZapFields should be called while Stats is locked
func (s *Stats) getZapFields(meter dmetering.Meter) []zap.Field {
	// Logging fields order is important as it affects the final rendering, we carefully ordered
	// them so the development logs looks nicer.
	tier := "tier1"
	if s.config.Tier2 {
		tier = "tier2"
	}
	errorText := ""
	if s.error != nil {
		errorText = s.error.Error()
	}

	// WARNING: DO NOT MODIFY THESE FIELDS, THEY ARE USED IN REPORTING SYSTEMS ##################
	out := []zap.Field{
		zap.String("user_id", s.config.UserID),
		zap.String("api_key_id", s.config.ApiKeyID),
		zap.String("output_module_name", s.config.OutputModule),
		zap.String("output_module_hash", s.config.OutputModuleHash),
		zap.Bool("production_mode", s.config.ProductionMode),
		zap.String("tier", tier),
		zap.String("block_rate_per_sec", s.blockRate.RateString()),
		zap.Uint64("local_blocks_processed", s.blockRate.Total()),
		zap.Duration("parallel_duration", s.initDuration),
		zap.Duration("module_exec_duration", s.moduleExecDuration()),
		zap.Duration("module_wasm_ext_duration", s.moduleWasmExtDuration()),
		zap.Duration("time_to_first_data", s.timeToFirstData),
		zap.Uint64("remote_jobs_completed", s.completedJobs),
		zap.Uint64("remote_jobs_failed", s.failedJobs),
		zap.Uint64("remote_jobs_incomplete", s.startedJobs-s.completedJobs-s.failedJobs), // either canceled or not returned yet (will be definitely be canceled soon!)
		zap.Uint64("remote_jobs_retried", s.retriedJobs),
		zap.Uint64("remote_jobs_delayed", s.delayedJobs),
		zap.Uint64("remote_blocks_processed", s.remoteProcessedBlockCount), // "estimated" from remote ranges
		zap.Uint64("total_blocks_processed", s.processedBlocks.Load()),     // includes remote and local blocks processed in this request, multiplied by execution stages, excludes blocks that were skipped from indexes
		zap.Uint64("uncompressed_egress_bytes", s.uncompressedEgressBytes),
		zap.Duration("client_read_average_time_last_5_minutes", s.clientReadTime.Average()),
		zap.Uint64("last_sent_block_num", s.lastSentBlockNum),
		zap.String("last_sent_block_id", s.lastSentBlockID),
		zap.Time("last_sent_block_time", s.lastSentBlockTime),
		zap.String("error", errorText),
	}

	var storeSizes []*storeSize
	for _, stats := range s.modulesStats {
		found := false
		for _, module := range s.stores {
			if module.Name == stats.Name {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		ss := &storeSize{
			name: stats.Name,
			hash: s.moduleHashes[stats.Name],
			size: stats.StoreSizeBytes,
		}
		storeSizes = append(storeSizes, ss)
	}

	out = append(out, zap.Objects("store_sizes", storeSizes))

	if meter != nil {
		remoteBytesRead, _ := s.remoteBytesConsumption()
		out = append(out, zap.Uint64("total_uncompressed_read_bytes", remoteBytesRead+uint64(meter.GetCount("total_read_bytes"))))
	}
	// ##########################################################################################

	// Additive only: `module_wasm_ext_duration` above merges every extension into a single
	// duration, which tells you that external calls are slow but not which one. The global list
	// answers "which extension", the per-module list answers "which module to go fix". Both are
	// empty when no extension call happened.
	byModule := s.wasmExtensionCallMetricsByModule()
	out = append(out, zap.Objects("wasm_ext_call_metrics", aggregateWasmExtensionCallMetricsByExtension(byModule)))
	out = append(out, zap.Objects("wasm_ext_call_metrics_by_module", byModule))

	return out
}

// wasmExtensionCallMetric is one row of the external call (e.g. eth_call) breakdown reported in the
// final request stats log. When `module` is empty the row is the aggregate across every module for
// a given extension; otherwise it is scoped to that module.
type wasmExtensionCallMetric struct {
	module    string
	extension string
	count     uint64
	totalTime time.Duration
	// maxTime is only known for calls made locally by this process. Calls made by tier2 jobs are
	// reported back as a count and a total only, and each tier2 logs its own max.
	maxTime time.Duration
	// inFlight, oldestInFlight and oldestInFlightBlock cover calls that started and have not
	// returned. Same caveat as maxTime: only locally executed modules are visible here, a
	// tier2 job folds the elapsed time of its own in-flight calls into the total it reports.
	inFlight            uint64
	oldestInFlight      time.Duration
	oldestInFlightBlock uint64
	// failed counts the calls that came back with an error, wherever they ran.
	failed uint64
}

func (m *wasmExtensionCallMetric) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if m.module != "" {
		encoder.AddString("module", m.module)
	}
	encoder.AddString("extension", m.extension)
	encoder.AddUint64("count", m.count)
	encoder.AddInt64("total_ms", m.totalTime.Milliseconds())

	var averageMs float64
	if m.count > 0 {
		averageMs = float64((m.totalTime / time.Duration(m.count)).Microseconds()) / 1000
	}
	encoder.AddFloat64("avg_ms", averageMs)
	encoder.AddInt64("max_ms", m.maxTime.Milliseconds())

	return nil
}

// wasmExtensionCallMetricsByModule returns one entry per (module, extension) pair that actually
// made at least one external call, aggregated across the same three sources moduleWasmExtDuration
// sums: locally executed modules, running jobs and completed jobs. Modules with no external call
// are absent, so the list is empty when nothing called out.
//
// wasmExtensionCallMetricsByModule should be called while Stats is locked
func (s *Stats) wasmExtensionCallMetricsByModule() []*wasmExtensionCallMetric {
	// module -> extension -> metric
	byModule := make(map[string]map[string]*wasmExtensionCallMetric)

	metricFor := func(module, extension string) *wasmExtensionCallMetric {
		extensions, ok := byModule[module]
		if !ok {
			extensions = make(map[string]*wasmExtensionCallMetric)
			byModule[module] = extensions
		}
		metric, ok := extensions[extension]
		if !ok {
			metric = &wasmExtensionCallMetric{module: module, extension: extension}
			extensions[extension] = metric
		}
		return metric
	}

	// Only the local metrics carry a per-call max, the remote ones are counts and totals.
	for module, mod := range s.modulesStats {
		for extension, callMetric := range mod.externalCallMetrics {
			metric := metricFor(module, extension)
			metric.count += callMetric.count
			metric.totalTime += callMetric.time
			metric.failed += callMetric.failed
			if callMetric.maxTime > metric.maxTime {
				metric.maxTime = callMetric.maxTime
			}
		}
		// A call that is still running has already been counted (the count is incremented when
		// it starts) but has contributed no time yet. Left out, a call hung for minutes against
		// a dead endpoint looks instantaneous, which is the opposite of what is happening.
		for _, inProcess := range mod.inprocessCallMetrics {
			metric := metricFor(module, inProcess.extension)
			elapsed := time.Since(inProcess.startTime)
			metric.totalTime += elapsed
			metric.inFlight++
			if elapsed > metric.oldestInFlight {
				metric.oldestInFlight = elapsed
				metric.oldestInFlightBlock = inProcess.blockNum
			}
		}
	}

	addRemote := func(modulesStats map[string]*pbssinternal.ModuleStats) {
		for module, mod := range modulesStats {
			for _, callMetric := range mod.ExternalCallMetrics {
				metric := metricFor(module, callMetric.Name)
				metric.count += callMetric.Count
				metric.totalTime += time.Duration(callMetric.TimeMs) * time.Millisecond
				metric.failed += callMetric.FailedCount
				metric.inFlight += callMetric.InFlightCount
				if oldest := time.Duration(callMetric.OldestInFlightMs) * time.Millisecond; oldest > metric.oldestInFlight {
					metric.oldestInFlight = oldest
					metric.oldestInFlightBlock = callMetric.OldestInFlightBlock
				}
			}
		}
	}

	for _, job := range s.runningJobs {
		addRemote(job.modulesStats)
	}
	addRemote(s.completedJobsStats)

	var out []*wasmExtensionCallMetric
	for _, extensions := range byModule {
		for _, metric := range extensions {
			out = append(out, metric)
		}
	}
	slices.SortFunc(out, func(a, b *wasmExtensionCallMetric) int {
		return cmp.Or(
			strings.Compare(a.module, b.module),
			strings.Compare(a.extension, b.extension),
		)
	})

	return out
}

// aggregateWasmExtensionCallMetricsByExtension collapses the per-module breakdown into one entry
// per extension (module left empty), for the quick-glance global view.
func aggregateWasmExtensionCallMetricsByExtension(byModule []*wasmExtensionCallMetric) []*wasmExtensionCallMetric {
	byExtension := make(map[string]*wasmExtensionCallMetric)

	for _, m := range byModule {
		metric, ok := byExtension[m.extension]
		if !ok {
			metric = &wasmExtensionCallMetric{extension: m.extension}
			byExtension[m.extension] = metric
		}
		metric.count += m.count
		metric.totalTime += m.totalTime
		if m.maxTime > metric.maxTime {
			metric.maxTime = m.maxTime
		}
	}

	out := make([]*wasmExtensionCallMetric, 0, len(byExtension))
	for _, metric := range byExtension {
		out = append(out, metric)
	}
	slices.SortFunc(out, func(a, b *wasmExtensionCallMetric) int {
		return strings.Compare(a.extension, b.extension)
	})

	return out
}

// moduleExecDuration should be called while Stats is locked
func (s *Stats) moduleExecDuration() (out time.Duration) {
	for _, m := range s.modulesStats {
		out += m.processingTime
	}
	return
}

// moduleWasmExtDuration should be called while Stats is locked
func (s *Stats) moduleWasmExtDuration() (out time.Duration) {
	for _, m := range s.modulesStats {
		for _, mm := range m.externalCallMetrics {
			out += mm.time
		}
	}
	for _, j := range s.runningJobs {
		for _, m := range j.modulesStats {
			for _, mm := range m.ExternalCallMetrics {
				out += time.Duration(mm.TimeMs) * time.Millisecond
			}
		}
	}
	for _, m := range s.completedJobsStats {
		for _, mm := range m.ExternalCallMetrics {
			out += time.Duration(mm.TimeMs) * time.Millisecond
		}
	}

	return
}
