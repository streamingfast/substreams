package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWasmMetricsGatherer_RecordsDoneCalls(t *testing.T) {
	gatherer := &WasmMetricsGatherer{logger: zlogTest}

	id := gatherer.RecordModuleWasmExternalCallBegin("mod", "eth:call", 12_450_739)
	gatherer.RecordModuleWasmExternalCallEnd("mod", "eth:call", id, nil)

	require.Contains(t, gatherer.doneCalls, "mod")
	require.Contains(t, gatherer.doneCalls["mod"], "eth:call")
	assert.Equal(t, uint64(1), gatherer.doneCalls["mod"]["eth:call"].Count)
	assert.Equal(t, "eth:call", gatherer.doneCalls["mod"]["eth:call"].Name)

	// A second call on the same (module, extension) must accumulate onto the same metric.
	id = gatherer.RecordModuleWasmExternalCallBegin("mod", "eth:call", 12_450_739)
	gatherer.RecordModuleWasmExternalCallEnd("mod", "eth:call", id, nil)

	assert.Equal(t, uint64(2), gatherer.doneCalls["mod"]["eth:call"].Count)
	assert.Len(t, gatherer.doneCalls["mod"], 1)
}

func TestWasmMetricsGatherer_ApplyToStats(t *testing.T) {
	gatherer := &WasmMetricsGatherer{logger: zlogTest}

	for range 3 {
		id := gatherer.RecordModuleWasmExternalCallBegin("mod", "eth:call", 12_450_739)
		gatherer.RecordModuleWasmExternalCallEnd("mod", "eth:call", id, nil)
	}
	id := gatherer.RecordModuleWasmExternalCallBegin("mod", "eth:balance", 12_450_739)
	gatherer.RecordModuleWasmExternalCallEnd("mod", "eth:balance", id, nil)

	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	gatherer.ApplyToStats(stats)

	callMetrics := stats.modulesStats["mod"].externalCallMetrics
	require.Contains(t, callMetrics, "eth:call")
	require.Contains(t, callMetrics, "eth:balance")
	assert.Equal(t, uint64(3), callMetrics["eth:call"].count)
	assert.Equal(t, uint64(1), callMetrics["eth:balance"].count)
}

// The gatherer is built as `&metrics.WasmMetricsGatherer{}` by the shared cache, so it has a nil
// logger. An unmatched End must not panic on it.
func TestWasmMetricsGatherer_EndWithoutBeginNilLogger(t *testing.T) {
	gatherer := &WasmMetricsGatherer{}

	assert.NotPanics(t, func() {
		gatherer.RecordModuleWasmExternalCallEnd("mod", "eth:call", 42, nil)
	})
	assert.Empty(t, gatherer.doneCalls)
}
