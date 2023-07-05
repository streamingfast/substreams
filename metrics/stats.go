package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"go.uber.org/zap"
)

type Stats interface {
	RecordParallelDuration(elapsed time.Duration)
	RecordWasmExtDuration(wasmExtName string, elapsed time.Duration)
	RecordModuleExecDuration(elapsed time.Duration)
	RecordBlock(ref bstream.BlockRef)
	LogAndClose()
}

func NewNoopStats() Stats {
	return &noopstats{}
}

type noopstats struct{}

func (n noopstats) RecordParallelDuration(_ time.Duration)          {}
func (n noopstats) RecordBlock(_ bstream.BlockRef)                  {}
func (n noopstats) RecordWasmExtDuration(_ string, _ time.Duration) {}
func (n noopstats) RecordModuleExecDuration(_ time.Duration)        {}
func (n noopstats) LogAndClose()                                    {}

type Config struct {
	UserID           string
	ApiKeyID         string
	OutputModule     string
	OutputModuleHash string
	ProductionMode   bool
	Tier2            bool
}

func NewReqStats(config *Config, logger *zap.Logger) Stats {
	return &stats{
		config:           config,
		blockRate:        dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		wasmExtDurations: map[string]time.Duration{},
		llDuration:       0,
		logger:           logger,
	}
}

type stats struct {
	config     *Config
	blockRate  *dmetrics.AvgRateCounter
	llDuration time.Duration

	wasmExtDurationLock sync.RWMutex
	wasmExtDurations    map[string]time.Duration
	moduleExecDuration  time.Duration

	logger *zap.Logger
}

func (s *stats) RecordParallelDuration(elapsed time.Duration) {
	s.llDuration = elapsed
}

func (s *stats) RecordBlock(ref bstream.BlockRef) {
	s.blockRate.Add(1)
}

func (s *stats) RecordWasmExtDuration(wasmExtName string, elapsed time.Duration) {
	s.wasmExtDurationLock.Lock()
	defer s.wasmExtDurationLock.Unlock()
	s.wasmExtDurations[wasmExtName] += elapsed
}

func (s *stats) RecordModuleExecDuration(elapsed time.Duration) {
	s.moduleExecDuration += elapsed
}

func (s *stats) LogAndClose() {
	s.blockRate.SyncNow()
	s.blockRate.Stop()
	s.logger.Info("substreams request stats", s.getZapFields()...)
}

func (s *stats) getZapFields() []zap.Field {
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
		zap.Duration("parallel_duration", s.llDuration),
		zap.Duration("module_exec_duration", s.moduleExecDuration),
	}

	for name, duration := range s.wasmExtDurations {
		out = append(out, zap.Duration(fmt.Sprintf("%s_wasm_ext_duration", name), duration))
	}
	return out
}
