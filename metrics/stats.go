package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type Stats struct {
	sync.Mutex

	config *Config

	blockRate *dmetrics.AvgRateCounter

	startTime    time.Time
	stages       []*pbsubstreamsrpc.Stage
	initDuration time.Duration

	// moduleStats only contain stats from local execution
	modulesStats map[string]*extendedStats

	runningJobs        runningJobs
	completedJobsStats map[string]*pbssinternal.ModuleStats

	localProcessedBlockCount  uint64
	completedJobsBytesRead    uint64
	completedJobsBytesWritten uint64

	// counter is used to get the next jobIdx
	counter uint64

	logger *zap.Logger
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
		count += job.ProcessedBlocks
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
	job.ProcessedBlocks = upd.ProcessedBlocks
	job.DurationMs = upd.DurationMs
	job.bytesRead = upd.TotalBytesRead
	job.bytesWritten = upd.TotalBytesWritten
}

func NewReqStats(config *Config, logger *zap.Logger) *Stats {
	return &Stats{
		config:             config,
		blockRate:          dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		startTime:          time.Now(),
		logger:             logger,
		modulesStats:       make(map[string]*extendedStats),
		runningJobs:        make(map[uint64]*extendedJob),
		completedJobsStats: make(map[string]*pbssinternal.ModuleStats),
	}
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
}

type extendedCallMetric struct {
	count uint64
	time  time.Duration
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
			Name:   k,
			Count:  v.count,
			TimeMs: uint64(v.time.Milliseconds()),
		}
		for _, inproc := range s.inprocessCallMetrics {
			if inproc.extension == k {
				callMetric.TimeMs += uint64(time.Since(inproc.startTime).Milliseconds())
			}
		}

		s.ModuleStats.ExternalCallMetrics[i] = callMetric
		sort.Slice(s.ModuleStats.ExternalCallMetrics, func(i, j int) bool {
			return s.ModuleStats.ExternalCallMetrics[i].Name < s.ModuleStats.ExternalCallMetrics[j].Name
		})
		i++
	}
	s.ModuleStats.StoreOperationTimeMs = uint64(s.storeOperationTime.Milliseconds())
}

func (s *Stats) RecordInitializationComplete() {
	s.Lock()
	defer s.Unlock()
	s.initDuration = time.Since(s.startTime)
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

	s.runningJobs[id] = &extendedJob{
		start: time.Now(),
		Job: &pbsubstreamsrpc.Job{
			Stage:           stage,
			StartBlock:      startBlock,
			StopBlock:       stopBlock,
			ProcessedBlocks: 0,
			DurationMs:      0,
		},
		modulesStats: make(map[string]*pbssinternal.ModuleStats),
	}
	s.Unlock()
	return id
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

func (s *Stats) RecordEndSubrequest(jobIdx uint64) {
	s.Lock()
	defer s.Unlock()
	job := s.runningJobs[jobIdx]

	for i := 0; i <= int(job.Stage); i++ {
		for _, mod := range s.stages[i].Modules {
			if _, ok := s.modulesStats[mod]; !ok {
				s.modulesStats[mod] = newExtendedStats(mod)
			}
			s.modulesStats[mod].processedBlocksInCompleteJobs += job.ProcessedBlocks
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
func (s *Stats) RecordModuleWasmExternalCallBegin(moduleName string, extension string) uint64 {
	s.Lock()
	defer s.Unlock()

	mod := s.moduleStats(moduleName)
	uniqueID := uniqueIDCounter.Inc()

	// initialize map
	mod.inprocessCallMetrics[uniqueID] = inprocessCall{
		startTime: time.Now(),
		extension: extension,
	}

	return uniqueID
}

// RecordModuleWasmExternalCallEnd can be called multiple times per module per block, for each external module call (ex: eth_call). `elapsed` is the time spent in executing that call.
func (s *Stats) RecordModuleWasmExternalCallEnd(moduleName string, extension string, uniqueID uint64) {
	s.Lock()
	defer s.Unlock()

	mod := s.moduleStats(moduleName)
	met, ok := mod.externalCallMetrics[extension]
	if !ok {
		met = &extendedCallMetric{}
		mod.externalCallMetrics[extension] = met
	}
	met.count++
	inproc := mod.inprocessCallMetrics[uniqueID]
	met.time += time.Since(inproc.startTime)

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

func (s *Stats) RecordBlock(ref bstream.BlockRef) {
	s.Lock()
	defer s.Unlock()
	s.blockRate.Add(1)
	s.localProcessedBlockCount += 1
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
			Stage:           v.Stage,
			StartBlock:      v.StartBlock,
			StopBlock:       v.StopBlock,
			ProcessedBlocks: v.ProcessedBlocks,
			DurationMs:      uint64(time.Since(v.start).Milliseconds()),
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
		for _, mod := range ss.Modules {
			if mod == module {
				return uint32(i), ss
			}
		}
	}
	// could happen on initial lookup, minor race condition
	return 0, nil
}

func (s *Stats) RemoteBytesConsumption() (read uint64, written uint64) {
	s.Lock()
	defer s.Unlock()
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

func (s *Stats) LogAndClose() {
	s.Lock()
	defer s.Unlock()
	s.blockRate.SyncNow()
	s.blockRate.Stop()
	s.logger.Info("substreams request stats", s.getZapFields()...)
}

// getZapFields should be called while Stats is locked
func (s *Stats) getZapFields() []zap.Field {
	// Logging fields order is important as it affects the final rendering, we carefully ordered
	// them so the development logs looks nicer.
	tier := "tier1"
	if s.config.Tier2 {
		tier = "tier2"
	}

	out := []zap.Field{
		zap.String("user_id", s.config.UserID),
		zap.String("api_key_id", s.config.ApiKeyID),
		zap.String("output_module_name", s.config.OutputModule),
		zap.String("output_module_hash", s.config.OutputModuleHash),
		zap.Bool("production_mode", s.config.ProductionMode),
		zap.String("tier", tier),
		zap.String("block_rate_per_sec", s.blockRate.RateString()),
		zap.Uint64("block_count", s.blockRate.Total()),
		zap.Duration("parallel_duration", s.initDuration),
		zap.Duration("module_exec_duration", s.moduleExecDuration()),
		zap.Duration("module_wasm_ext_duration", s.moduleWasmExtDuration()),
	}

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
