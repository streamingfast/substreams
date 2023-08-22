package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap"
)

type Stats struct {
	sync.Mutex

	config *Config

	blockRate *dmetrics.AvgRateCounter

	startTime      time.Time
	stages         []*pbsubstreamsrpc.Stage
	initDuration   time.Duration
	modulesStats   map[string]*extendedStats
	schedulerStats *schedulerStats
	counter        uint64

	logger *zap.Logger
}

type schedulerStats struct {
	runningJobs map[uint64]*extendedJob
}

func (s *Stats) ApplyTier2Update(jobIdx uint64, upd *pbssinternal.Update) {
	s.Lock()
	defer s.Unlock()
	s.schedulerStats.runningJobs[jobIdx].ProcessedBlocks = upd.ProcessedBlocks
	for _, modStatUpdate := range upd.ModulesStats {
		modStat, ok := s.modulesStats[modStatUpdate.Name]
		if !ok {
			s.modulesStats[modStatUpdate.Name] = &extendedStats{
				ModuleStats: modStatUpdate,
			}
			continue
		}

		modStat.StoreReadCount += modStatUpdate.StoreReadCount
		modStat.ProcessingTimeMs += modStatUpdate.ProcessingTimeMs
		modStat.StoreDeleteprefixCount += modStatUpdate.StoreDeleteprefixCount
		modStat.StoreWriteCount += modStatUpdate.StoreWriteCount
		modStat.StoreOperationTimeMs += modStatUpdate.StoreOperationTimeMs
		if modStatUpdate.StoreSizeBytes > modStat.StoreSizeBytes {
			modStat.StoreSizeBytes = modStatUpdate.StoreSizeBytes
		}
		for _, v := range modStatUpdate.ExternalCallMetrics {
			var found bool
			for _, prev := range modStat.ExternalCallMetrics {
				if prev.Name == v.Name {
					found = true
					prev.Count += v.Count
					prev.TimeMs += v.TimeMs
				}
			}
			if !found {
				modStat.ExternalCallMetrics = append(modStat.ExternalCallMetrics, v)
			}
		}
	}
}

func NewReqStats(config *Config, logger *zap.Logger) *Stats {
	return &Stats{
		config:       config,
		blockRate:    dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		startTime:    time.Now(),
		logger:       logger,
		modulesStats: make(map[string]*extendedStats),
		schedulerStats: &schedulerStats{
			runningJobs: make(map[uint64]*extendedJob),
		},
	}
}

type extendedStats struct {
	*pbssinternal.ModuleStats

	merging                       bool
	processedBlocksInCompleteJobs uint64

	storeOperationTime  time.Duration
	processingTime      time.Duration
	externalCallTime    time.Duration
	externalCallMetrics map[string]*extendedCallMetric
}

type extendedCallMetric struct {
	count uint64
	time  time.Duration
}

// updateDurations should be called while locked
func (s *extendedStats) updateDurations() {
	s.ModuleStats.ProcessingTimeMs = uint64(s.processingTime.Milliseconds())
	s.ModuleStats.ExternalCallMetrics = make([]*pbssinternal.ExternalCallMetric, len(s.externalCallMetrics))
	i := 0
	for k, v := range s.externalCallMetrics {
		s.ModuleStats.ExternalCallMetrics[i] = &pbssinternal.ExternalCallMetric{
			Name:   k,
			Count:  v.count,
			TimeMs: uint64(v.time.Milliseconds()),
		}
		sort.Slice(s.ModuleStats.ExternalCallMetrics, func(i, j int) bool {
			return s.ModuleStats.ExternalCallMetrics[i].Name < s.ModuleStats.ExternalCallMetrics[j].Name
		})
		i++
	}
	s.ModuleStats.StoreOperationTimeMs = uint64(s.storeOperationTime.Milliseconds())
}

type extendedJob struct {
	*pbsubstreamsrpc.Job
	start time.Time
}

func (s *Stats) RecordInitializationComplete() {
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

	s.schedulerStats.runningJobs[id] = &extendedJob{
		start: time.Now(),
		Job: &pbsubstreamsrpc.Job{
			Stage:           stage,
			StartBlock:      startBlock,
			StopBlock:       stopBlock,
			ProcessedBlocks: 0,
			DurationMs:      0,
		},
	}
	s.Unlock()
	return id
}

func (s *Stats) RecordEndSubrequest(id uint64) {
	s.Lock()
	job := s.schedulerStats.runningJobs[id]

	stage := s.stages[job.Stage]
	for _, mod := range stage.Modules {
		s.modulesStats[mod].processedBlocksInCompleteJobs += job.ProcessedBlocks
	}

	delete(s.schedulerStats.runningJobs, id)
	s.Unlock()
}

// RecordModuleWasmBlock should be called once per module per block. `elapsed` is the time spent in executing the WASM code, including store and extension calls
func (s *Stats) RecordModuleWasmBlock(moduleName string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.processingTime += elapsed
}

// RecordModuleWasmExternalCall can be called multiple times per module per block, for each external module call (ex: eth_call). `elapsed` is the time spent in executing that call.
func (s *Stats) RecordModuleWasmExternalCall(moduleName string, extension string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)

	met, ok := mod.externalCallMetrics[extension]
	if !ok {
		met = &extendedCallMetric{}
		mod.externalCallMetrics[extension] = met
	}
	met.count++
	met.time += elapsed
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
	s.blockRate.Add(1)
}

// moduleStats should be called while locked
func (s *Stats) moduleStats(moduleName string) *extendedStats {
	mod, ok := s.modulesStats[moduleName]
	if !ok {
		mod = &extendedStats{
			ModuleStats: &pbssinternal.ModuleStats{
				Name: moduleName,
			},
			externalCallMetrics: make(map[string]*extendedCallMetric),
		}
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

	out := make([]*pbsubstreamsrpc.Job, len(s.schedulerStats.runningJobs))
	i := 0
	for _, v := range s.schedulerStats.runningJobs {
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

func (s *Stats) ModulesStats() []*pbssinternal.ModuleStats {
	s.Lock()
	defer s.Unlock()

	out := make([]*pbssinternal.ModuleStats, len(s.modulesStats))
	i := 0
	for _, v := range s.modulesStats {
		v.updateDurations()
		out[i] = v.ModuleStats
		i++
	}

	return out
}

func (s *Stats) Stage(module string) *pbsubstreamsrpc.Stage {
	for _, ss := range s.stages {
		for _, mod := range ss.Modules {
			if mod == module {
				return ss
			}
		}
	}
	// could happen on initial lookup, minor race condition
	return nil
}

func (s *Stats) AggregatedModulesStats() []*pbsubstreamsrpc.ModuleStats {
	s.Lock()
	defer s.Unlock()

	out := make([]*pbsubstreamsrpc.ModuleStats, len(s.modulesStats))
	i := 0
	for _, v := range s.modulesStats {
		out[i] = &pbsubstreamsrpc.ModuleStats{
			Name:                        v.Name,
			TotalProcessedBlockCount:    v.processedBlocksInCompleteJobs, // FIXME: add the ones in jobs
			TotalProcessingTimeMs:       v.ProcessingTimeMs,
			ExternalCallMetrics:         toRPCExternalCallMetrics(v.ExternalCallMetrics),
			StoreSizeBytes:              v.StoreSizeBytes,
			TotalStoreOperationTimeMs:   v.StoreOperationTimeMs,
			TotalStoreReadCount:         v.StoreReadCount,
			TotalStoreWriteCount:        v.StoreWriteCount,
			TotalStoreDeleteprefixCount: v.StoreDeleteprefixCount,
			StoreCurrentlyMerging:       v.merging,
			// TotalStoreMergingTimeMs: //FIXME .. need to store this
			// TotalErrorCount: v.ModuleStats. //FIXME .. need to store this
		}
		if stage := s.Stage(v.Name); stage != nil { // will be nil for mappers
			if ranges := stage.CompletedRanges; ranges != nil {
				out[i].HighestContiguousBlock = ranges[0].EndBlock
			}
		}
		i++
	}

	return out
}

func toRPCExternalCallMetrics(in []*pbssinternal.ExternalCallMetric) []*pbsubstreamsrpc.ExternalCallMetric {
	if in == nil {
		return nil
	}

	out := make([]*pbsubstreamsrpc.ExternalCallMetric, len(in))
	for i := range in {
		out[i] = &pbsubstreamsrpc.ExternalCallMetric{
			Name:   in[i].Name,
			Count:  in[i].Count,
			TimeMs: in[i].TimeMs,
		}
	}
	return out
}

func (s *Stats) LogAndClose() {
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
		out += m.externalCallTime
	}
	return
}
