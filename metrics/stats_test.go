package metrics

import (
	"testing"
	"time"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestStats_WasmExtensionCallMetrics_AggregatesAcrossModules(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)

	// The same extension called from two different modules must collapse into a single entry in the
	// global view, since what is being hunted first is the slow extension, not the module.
	setExternalCallMetric(stats, "mod_a", "eth:call", 2, 400*time.Millisecond, 300*time.Millisecond)
	setExternalCallMetric(stats, "mod_b", "eth:call", 1, 200*time.Millisecond, 200*time.Millisecond)
	setExternalCallMetric(stats, "mod_b", "eth:balance", 1, 50*time.Millisecond, 50*time.Millisecond)

	metrics := aggregateWasmExtensionCallMetricsByExtension(stats.wasmExtensionCallMetricsByModule())
	require.Len(t, metrics, 2)

	// Sorted by extension name, module left empty in the global view.
	assert.Equal(t, "eth:balance", metrics[0].extension)
	assert.Empty(t, metrics[0].module)
	assert.Equal(t, "eth:call", metrics[1].extension)

	ethCall := metrics[1]
	assert.Equal(t, uint64(3), ethCall.count)
	assert.Equal(t, 600*time.Millisecond, ethCall.totalTime)
	assert.Equal(t, 300*time.Millisecond, ethCall.maxTime)
}

func TestStats_WasmExtensionCallMetricsByModule(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)

	setExternalCallMetric(stats, "mod_a", "eth:call", 2, 400*time.Millisecond, 300*time.Millisecond)
	setExternalCallMetric(stats, "mod_b", "eth:call", 1, 200*time.Millisecond, 200*time.Millisecond)

	metrics := stats.wasmExtensionCallMetricsByModule()
	require.Len(t, metrics, 2)

	// Sorted by module then extension, each entry scoped to its module.
	assert.Equal(t, "mod_a", metrics[0].module)
	assert.Equal(t, "eth:call", metrics[0].extension)
	assert.Equal(t, uint64(2), metrics[0].count)
	assert.Equal(t, 300*time.Millisecond, metrics[0].maxTime)

	assert.Equal(t, "mod_b", metrics[1].module)
	assert.Equal(t, uint64(1), metrics[1].count)
}

func TestStats_WasmExtensionCallMetrics_IncludesRemoteJobs(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)

	setExternalCallMetric(stats, "mod_a", "eth:call", 1, 100*time.Millisecond, 100*time.Millisecond)

	stats.completedJobsStats["mod_a"] = &pbssinternal.ModuleStats{
		Name: "mod_a",
		ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{
			{Name: "eth:call", Count: 4, TimeMs: 400},
		},
	}
	stats.runningJobs[0] = &extendedJob{
		Job: &pbsubstreamsrpc.Job{},
		modulesStats: map[string]*pbssinternal.ModuleStats{
			"mod_a": {
				Name: "mod_a",
				ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{
					{Name: "eth:call", Count: 2, TimeMs: 200},
				},
			},
		},
	}

	metrics := aggregateWasmExtensionCallMetricsByExtension(stats.wasmExtensionCallMetricsByModule())
	require.Len(t, metrics, 1)

	assert.Equal(t, uint64(7), metrics[0].count)
	assert.Equal(t, 700*time.Millisecond, metrics[0].totalTime)
	// Remote jobs report a count and a total only, so the max stays the local one.
	assert.Equal(t, 100*time.Millisecond, metrics[0].maxTime)
}

func TestStats_WasmExtensionCallMetrics_Empty(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)

	assert.Empty(t, stats.wasmExtensionCallMetricsByModule())
	assert.Empty(t, aggregateWasmExtensionCallMetricsByExtension(stats.wasmExtensionCallMetricsByModule()))
}

// RecordModuleWasmExternalCallEnd is what actually feeds maxTime, so exercise it for real rather
// than only asserting on hand-built state.
func TestStats_RecordModuleWasmExternalCallEnd_TracksMax(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)

	slowID := stats.RecordModuleWasmExternalCallBegin("mod_a", "eth:call")
	time.Sleep(30 * time.Millisecond)
	stats.RecordModuleWasmExternalCallEnd("mod_a", "eth:call", slowID)

	slowest := stats.modulesStats["mod_a"].externalCallMetrics["eth:call"].maxTime
	require.GreaterOrEqual(t, slowest, 30*time.Millisecond)

	// A faster call afterwards must not lower the recorded max.
	fastID := stats.RecordModuleWasmExternalCallBegin("mod_a", "eth:call")
	stats.RecordModuleWasmExternalCallEnd("mod_a", "eth:call", fastID)

	callMetric := stats.modulesStats["mod_a"].externalCallMetrics["eth:call"]
	assert.Equal(t, slowest, callMetric.maxTime)
	assert.Equal(t, uint64(2), callMetric.count)
}

func TestWasmExtensionCallMetric_MarshalLogObject(t *testing.T) {
	encoder := zapcore.NewMapObjectEncoder()

	metric := &wasmExtensionCallMetric{
		module:    "mod_a",
		extension: "eth:call",
		count:     4,
		totalTime: 1 * time.Second,
		maxTime:   700 * time.Millisecond,
	}
	require.NoError(t, metric.MarshalLogObject(encoder))

	assert.Equal(t, "mod_a", encoder.Fields["module"])
	assert.Equal(t, "eth:call", encoder.Fields["extension"])
	assert.Equal(t, uint64(4), encoder.Fields["count"])
	assert.Equal(t, int64(1000), encoder.Fields["total_ms"])
	assert.Equal(t, float64(250), encoder.Fields["avg_ms"])
	assert.Equal(t, int64(700), encoder.Fields["max_ms"])
}

func TestWasmExtensionCallMetric_MarshalLogObject_GlobalOmitsModule(t *testing.T) {
	encoder := zapcore.NewMapObjectEncoder()

	// A count of zero must not divide by zero, and the aggregate row carries no module.
	metric := &wasmExtensionCallMetric{extension: "eth:call"}
	require.NoError(t, metric.MarshalLogObject(encoder))

	assert.NotContains(t, encoder.Fields, "module")
	assert.Equal(t, float64(0), encoder.Fields["avg_ms"])
	assert.Equal(t, uint64(0), encoder.Fields["count"])
}

func setExternalCallMetric(stats *Stats, moduleName, extension string, count uint64, total, max time.Duration) {
	mod := stats.moduleStats(moduleName)
	mod.externalCallMetrics[extension] = &extendedCallMetric{count: count, time: total, maxTime: max}
}
