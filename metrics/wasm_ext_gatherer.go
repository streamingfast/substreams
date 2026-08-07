package metrics

import (
	"sync"
	"time"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"go.uber.org/zap"
)

// this is used to gather metrics, then merge them into the Stats struct
type WasmMetricsGatherer struct {
	sync.Mutex
	// module to ID to [start-time + extension]
	inProcessCalls map[string]map[uint64]*inprocessCall
	// module to extension to (duration+count)
	doneCalls map[string]map[string]*pbssinternal.ExternalCallMetric
	logger    *zap.Logger
}

func (m *WasmMetricsGatherer) RecordModuleWasmExternalCallBegin(moduleName string, extension string, blockNum uint64) uint64 {
	m.Lock()
	defer m.Unlock()
	if m.inProcessCalls == nil {
		m.inProcessCalls = make(map[string]map[uint64]*inprocessCall)
	}

	if m.inProcessCalls[moduleName] == nil {
		m.inProcessCalls[moduleName] = make(map[uint64]*inprocessCall)
	}

	uniqueID := uniqueIDCounter.Inc()
	m.inProcessCalls[moduleName][uniqueID] = &inprocessCall{
		startTime: time.Now(),
		extension: extension,
		blockNum:  blockNum,
	}

	return uniqueID
}

func (m *WasmMetricsGatherer) RecordModuleWasmExternalCallEnd(moduleName string, extension string, uniqueID uint64, callErr error) {
	m.Lock()
	defer m.Unlock()

	if m.inProcessCalls[moduleName] == nil || m.inProcessCalls[moduleName][uniqueID] == nil {
		if m.logger != nil {
			m.logger.Warn("inprocess call not found", zap.String("module", moduleName), zap.String("extension", extension), zap.Uint64("unique_id", uniqueID))
		}
		return
	}

	duration := time.Since(m.inProcessCalls[moduleName][uniqueID].startTime)
	delete(m.inProcessCalls[moduleName], uniqueID)

	if m.doneCalls == nil {
		m.doneCalls = make(map[string]map[string]*pbssinternal.ExternalCallMetric)
	}
	if m.doneCalls[moduleName] == nil {
		m.doneCalls[moduleName] = make(map[string]*pbssinternal.ExternalCallMetric)
	}

	metric, ok := m.doneCalls[moduleName][extension]
	if !ok {
		metric = &pbssinternal.ExternalCallMetric{
			Name:   extension,
			Count:  0,
			TimeMs: 0,
		}
		m.doneCalls[moduleName][extension] = metric
	}

	metric.Count++
	metric.TimeMs += uint64(duration.Milliseconds())
	if callErr != nil {
		metric.FailedCount++
	}
}

func (m *WasmMetricsGatherer) ApplyToStats(stats *Stats) {
	m.Lock()
	defer m.Unlock()

	stats.Lock()
	defer stats.Unlock()

	for moduleName, doneCalls := range m.doneCalls {
		metrics := stats.moduleStats(moduleName).externalCallMetrics
		for extension, metric := range doneCalls {
			if metrics[extension] == nil {
				metrics[extension] = &extendedCallMetric{}
			}
			metrics[extension].count += metric.Count
			metrics[extension].time += time.Duration(metric.TimeMs) * time.Millisecond
			metrics[extension].failed += metric.FailedCount
		}
	}

	// Calls that have not returned are the ones worth reporting the soonest, and they are the
	// ones this loop would otherwise drop: they sit in inProcessCalls until they complete, which
	// for a call retrying against a dead endpoint can be minutes.
	for moduleName, inProcess := range m.inProcessCalls {
		modStats := stats.moduleStats(moduleName)
		for uniqueID, call := range inProcess {
			modStats.inprocessCallMetrics[uniqueID] = *call
		}
	}
}

type WasmExtensionStats interface {
	RecordModuleWasmExternalCallBegin(moduleName string, extension string, blockNum uint64) uint64
	RecordModuleWasmExternalCallEnd(moduleName string, extension string, uniqueID uint64, callErr error)
}
