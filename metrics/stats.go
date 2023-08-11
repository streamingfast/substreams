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
	initDuration   time.Duration
	modulesStats   map[string]*extendedStats
	schedulerStats *schedulerStats
	counter        uint64

	logger *zap.Logger
}

type schedulerStats struct {
	runningJobs  map[uint64]*extendedJob
	modulesStats map[string]*pbssinternal.ModuleStats
}

func NewReqStats(config *Config, logger *zap.Logger) *Stats {
	return &Stats{
		config:         config,
		blockRate:      dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		startTime:      time.Now(),
		logger:         logger,
		modulesStats:   make(map[string]*extendedStats),
		schedulerStats: &schedulerStats{},
	}
}

type extendedStats struct {
	*pbssinternal.ModuleStats
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

func (s *Stats) RecordNewSubrequest(stage, startBlock, stopBlock uint64) (id uint64) {
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
