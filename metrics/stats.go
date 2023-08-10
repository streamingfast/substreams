package metrics

import (
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
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

	logger *zap.Logger
}

type schedulerStats struct {
	runningJobs int
}

func NewReqStats(config *Config, logger *zap.Logger) *Stats {
	return &Stats{
		config:       config,
		blockRate:    dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		startTime:    time.Now(),
		logger:       logger,
		modulesStats: make(map[string]*extendedStats),
		schedulerStats: &schedulerStats{
			runningJobs: 0,
		},
	}
}

type extendedStats struct {
	*pbssinternal.ModuleStats
	storeOperationsTime time.Duration
	processingTime      time.Duration
	externalCallsTime   time.Duration
}

func (s *extendedStats) updateDurations() {
	s.ModuleStats.ProcessingTimeMs = uint64(s.processingTime.Milliseconds())
	s.ModuleStats.ExternalCallsTimeMs = uint64(s.externalCallsTime.Milliseconds())
	s.ModuleStats.StoreOperationsTimeMs = uint64(s.storeOperationsTime.Milliseconds())
}

func (s *Stats) RecordInitializationComplete() {
	s.initDuration = time.Since(s.startTime)
}

// FIXME: add stage and range
func (s *Stats) RecordNewSubrequest() {
	s.Lock()
	s.schedulerStats.runningJobs += 1
	s.Unlock()
}
func (s *Stats) RecordEndSubrequest() {
	s.Lock()
	s.schedulerStats.runningJobs += 1
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
func (s *Stats) RecordModuleWasmExternalCall(moduleName string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.ExternalCallsCount++
	mod.externalCallsTime += elapsed
}

// RecordModuleWasmStoreRead can be called multiple times per module per block `elapsed` is the time spent in executing that operation.
func (s *Stats) RecordModuleWasmStoreRead(moduleName string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.StoreReadsCount++
	mod.storeOperationsTime += elapsed
}

// RecordModuleWasmStoreWrite can be called multiple times per module per block `elapsed` is the time spent in executing that operation.
func (s *Stats) RecordModuleWasmStoreWrite(moduleName string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.StoreWritesCount++
	mod.storeOperationsTime += elapsed
}

// RecordModuleWasmStoreDeletePrefix can be called multiple times per module per block `elapsed` is the time spent in executing that operation.
func (s *Stats) RecordModuleWasmStoreDeletePrefix(moduleName string, elapsed time.Duration) {
	s.Lock()
	defer s.Unlock()
	mod := s.moduleStats(moduleName)
	mod.StoreDeleteprefixCount++
	mod.storeOperationsTime += elapsed
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
		out += m.externalCallsTime
	}
	return
}
