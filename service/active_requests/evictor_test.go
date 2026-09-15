package active_requests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSelectVictims_ClassThenBurnOrder(t *testing.T) {
	candidates := []evictCandidate{
		{traceID: "prod-live-big", class: ClassProdLive, burnCores: 3.0},
		{traceID: "dev-small", class: ClassDev, burnCores: 0.2},
		{traceID: "dev-big", class: ClassDev, burnCores: 1.5},
		{traceID: "catchup", class: ClassProdCatchup, burnCores: 2.0},
	}

	// a small excess is covered by the least important class, even against a bigger burner
	victims := selectVictims(candidates, 1.0, DefaultEvictionOrder())
	require.Len(t, victims, 1)
	assert.Equal(t, "dev-big", victims[0].traceID)

	// cutting continues in class order until the evicted burn covers the excess
	victims = selectVictims(candidates, 3.0, DefaultEvictionOrder())
	require.Len(t, victims, 3)
	assert.Equal(t, "dev-big", victims[0].traceID)
	assert.Equal(t, "dev-small", victims[1].traceID)
	assert.Equal(t, "prod-live-big", victims[2].traceID)
}

func TestSelectVictims_CutsLiveBeforeCatchup(t *testing.T) {
	candidates := []evictCandidate{
		{traceID: "catchup-1", class: ClassProdCatchup, burnCores: 2.0},
		{traceID: "catchup-2", class: ClassProdCatchup, burnCores: 1.8},
		{traceID: "live-1", class: ClassProdLive, burnCores: 1.5},
	}

	// an excess nothing can cover takes every candidate, live before catchup
	victims := selectVictims(candidates, 100, DefaultEvictionOrder())
	require.Len(t, victims, 3)
	assert.Equal(t, "live-1", victims[0].traceID)
	assert.Equal(t, "catchup-1", victims[1].traceID)
	assert.Equal(t, "catchup-2", victims[2].traceID)

	// cutting stops as soon as the evicted burn covers the excess
	victims = selectVictims(candidates, 3.0, DefaultEvictionOrder())
	require.Len(t, victims, 2)
}

func TestSelectVictims_CustomOrderSkipsUnlistedClasses(t *testing.T) {
	t0 := time.Now()
	candidates := []evictCandidate{
		{traceID: "live", class: ClassProdLive, burnCores: 3.0},
		{traceID: "catchup", class: ClassProdCatchup, burnCores: 2.0},
		{traceID: "dev", class: ClassDev, burnCores: 0.5},
		{traceID: "cached-young", class: ClassProdCached, startTime: t0},
		{traceID: "cached-old", class: ClassProdCached, startTime: t0.Add(-time.Hour)},
	}
	order := []EvictionClass{ClassDev, ClassProdCached, ClassProdCatchup}

	// a prod-cached request's cost is unknown: cutting stops right after the first one, oldest first
	victims := selectVictims(candidates, 100, order)
	require.Len(t, victims, 2)
	assert.Equal(t, "dev", victims[0].traceID)
	assert.Equal(t, "cached-old", victims[1].traceID)

	// without prod-cached candidates, cutting goes on to catchup and never reaches live
	victims = selectVictims(candidates[:3], 100, order)
	require.Len(t, victims, 2)
	assert.Equal(t, "dev", victims[0].traceID)
	assert.Equal(t, "catchup", victims[1].traceID)
}

func TestParseEvictionOrder(t *testing.T) {
	order, err := ParseEvictionOrder("dev, prod-cached,prod-catchup")
	require.NoError(t, err)
	assert.Equal(t, []EvictionClass{ClassDev, ClassProdCached, ClassProdCatchup}, order)

	order, err = ParseEvictionOrder("")
	require.NoError(t, err)
	assert.Nil(t, order)

	_, err = ParseEvictionOrder("dev,prod")
	assert.ErrorContains(t, err, `invalid eviction class "prod"`)

	_, err = ParseEvictionOrder("dev,dev")
	assert.ErrorContains(t, err, "listed twice")
}

type fakeComputeStats struct{ compute time.Duration }

func (f *fakeComputeStats) LocalWasmComputeDuration() time.Duration { return f.compute }
func (f *fakeComputeStats) CurrentBlock() uint64                    { return 0 }

func TestSampleBurnRates_Classes(t *testing.T) {
	cfg := DefaultEvictorConfig()
	manager := NewActiveRequestsManager(zap.NewNop())
	ev := NewEvictor(cfg, manager, nil, zap.NewNop())
	noop := func(error) {}

	manager.Add(noop, "cached", "", 0, 0, 0, true, &fakeComputeStats{})
	catchupStats := &fakeComputeStats{}
	catchup := manager.Add(noop, "catchup", "", 0, 0, 0, true, catchupStats)
	catchup.SetProcessingBlocks()
	liveStats := &fakeComputeStats{}
	live := manager.Add(noop, "live", "", 0, 0, 0, true, liveStats)
	live.SetProcessingBlocks()
	live.SetLive()
	manager.Add(noop, "dev-idle", "", 0, 0, 0, false, &fakeComputeStats{})
	for _, req := range manager.reqs {
		req.StartTime = time.Now().Add(-time.Hour)
	}

	t0 := time.Now()
	ev.sampleBurnRates(t0)
	catchupStats.compute = 5 * time.Second
	liveStats.compute = 5 * time.Second
	candidates, total := ev.sampleBurnRates(t0.Add(5 * time.Second))
	assert.Equal(t, 4, total)

	classes := make(map[string]EvictionClass)
	for _, c := range candidates {
		classes[c.traceID] = c.class
	}
	// prod-cached burns no wasm but stays a candidate; an idle dev request does not
	assert.Equal(t, map[string]EvictionClass{
		"cached":  ClassProdCached,
		"catchup": ClassProdCatchup,
		"live":    ClassProdLive,
	}, classes)
}

func newTestEvictor(cfg EvictorConfig) *Evictor {
	return NewEvictor(cfg, nil, nil, zap.NewNop())
}

func TestClassify_SustainAndRecovery(t *testing.T) {
	cfg := DefaultEvictorConfig()
	cfg.Mode = EvictionFull // IsOverloaded only reports in an enforcing mode
	ev := newTestEvictor(cfg)
	t0 := time.Now()

	calm := CPUSignals{QuotaCores: 4, UsageRatio: 0.5}
	hot := CPUSignals{QuotaCores: 4, UsageRatio: 0.95}

	assert.False(t, ev.classify(calm, t0))
	assert.False(t, ev.IsOverloaded())

	// the overload fires only once the condition has held for Sustain
	assert.False(t, ev.classify(hot, t0))
	assert.False(t, ev.classify(hot, t0.Add(cfg.Sustain/2)))
	assert.False(t, ev.IsOverloaded())
	assert.True(t, ev.classify(hot, t0.Add(cfg.Sustain)))
	assert.True(t, ev.IsOverloaded())

	// a dip resets the sustain window
	assert.False(t, ev.classify(calm, t0.Add(cfg.Sustain+time.Second)))
	assert.False(t, ev.classify(hot, t0.Add(cfg.Sustain+2*time.Second)))

	// still overloaded until the recovery condition holds RecoverSustain
	assert.True(t, ev.IsOverloaded())
	assert.False(t, ev.classify(calm, t0.Add(time.Minute)))
	assert.True(t, ev.IsOverloaded())
	assert.False(t, ev.classify(calm, t0.Add(time.Minute+cfg.RecoverSustain)))
	assert.False(t, ev.IsOverloaded())

	// usage between RecoverThreshold and Threshold neither fires nor recovers
	ev = newTestEvictor(cfg)
	tepid := CPUSignals{QuotaCores: 4, UsageRatio: 0.85}
	assert.False(t, ev.classify(hot, t0))
	assert.True(t, ev.classify(hot, t0.Add(cfg.Sustain)))
	assert.False(t, ev.classify(tepid, t0.Add(time.Minute)))
	assert.True(t, ev.IsOverloaded())
}

func TestHoldSince(t *testing.T) {
	t0 := time.Now()
	assert.True(t, holdSince(time.Time{}, false, t0).IsZero())
	assert.Equal(t, t0, holdSince(time.Time{}, true, t0))
	assert.Equal(t, t0, holdSince(t0, true, t0.Add(time.Minute)))
	assert.True(t, holdSince(t0, false, t0.Add(time.Minute)).IsZero())
}

func TestEvictorConfig_WithDefaults(t *testing.T) {
	def := DefaultEvictorConfig()

	// zero config gets every tunable filled, mode stays off
	filled := EvictorConfig{}.WithDefaults()
	assert.Equal(t, EvictionOff, filled.Mode)
	filled.Mode = def.Mode
	assert.Equal(t, def, filled)

	// explicit values are kept
	custom := EvictorConfig{Mode: EvictionFull, TargetRatio: 0.6, Interval: time.Second}.WithDefaults()
	assert.Equal(t, EvictionFull, custom.Mode)
	assert.Equal(t, 0.6, custom.TargetRatio)
	assert.Equal(t, time.Second, custom.Interval)
	assert.Equal(t, def.Threshold, custom.Threshold)
}

func TestObserveModeDoesNotEnforce(t *testing.T) {
	cfg := DefaultEvictorConfig()
	cfg.Mode = EvictionObserve
	ev := newTestEvictor(cfg)
	t0 := time.Now()

	hot := CPUSignals{QuotaCores: 4, UsageRatio: 0.97}
	ev.classify(hot, t0)
	assert.True(t, ev.classify(hot, t0.Add(cfg.Sustain)))

	// internal state tracks the overload, but nothing visible to routing/admission
	assert.True(t, ev.overloaded.Load())
	assert.False(t, ev.IsOverloaded())
}

func TestEffectiveActiveRequests(t *testing.T) {
	cfg := DefaultEvictorConfig()
	cfg.NominalCapacity = 14
	cfg.TargetRatio = 0.75
	ev := newTestEvictor(cfg)

	tests := []struct {
		name           string
		activeRequests int
		usageRatio     float64
		expect         float64
	}{
		{"idle pod reports nothing", 0, 0, 0},
		{"cheap requests report their own count", 5, 0.10, 5},
		{"expensive requests report what they cost", 5, 0.30, 5.6},
		{"pod held at the eviction target reports full", 5, 0.75, 14},
		{"pod over the target reports more than full", 8, 0.95, 17.733333333333334},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ev.effectiveActiveRequests(CPUSignals{QuotaCores: 4, UsageRatio: tt.usageRatio}, tt.activeRequests)
			assert.InDelta(t, tt.expect, got, 0.0001)
		})
	}
}

func TestEffectiveActiveRequests_NoNominalCapacity(t *testing.T) {
	cfg := DefaultEvictorConfig()
	ev := newTestEvictor(cfg)
	assert.Equal(t, 6.0, ev.effectiveActiveRequests(CPUSignals{QuotaCores: 4, UsageRatio: 0.9}, 6))
}

func TestTick_UnreadableCgroupClearsOverload(t *testing.T) {
	dir := t.TempDir()
	writeCgroupFiles(t, dir, map[string]string{
		"cpu.max":  "400000 100000\n",
		"cpu.stat": "usage_usec 0\n",
	})
	reader, err := newCPUReaderAt(dir, 0)
	require.NoError(t, err)

	cfg := DefaultEvictorConfig()
	cfg.Mode = EvictionFull
	ev := NewEvictor(cfg, NewActiveRequestsManager(zap.NewNop()), reader, zap.NewNop())

	var evaluations int
	ev.OnEvaluate(func() { evaluations++ })

	t0 := time.Now()
	hot := CPUSignals{QuotaCores: 4, UsageRatio: 0.95}
	ev.classify(hot, t0)
	require.True(t, ev.classify(hot, t0.Add(cfg.Sustain)))
	require.True(t, ev.IsOverloaded())

	// the signal the overload was built from goes away: the pod cannot confirm it
	// is still overloaded, so it must go back to ready and accepting requests
	require.NoError(t, os.Remove(filepath.Join(dir, "cpu.stat")))
	ev.tick(t0.Add(cfg.Sustain + cfg.Interval))

	assert.False(t, ev.IsOverloaded())
	assert.Equal(t, 1, evaluations)
	assert.True(t, ev.overloadSince.IsZero())
	assert.True(t, ev.recoverSince.IsZero())
	assert.True(t, ev.unreadyAt.IsZero())

	// the readiness the host derives from IsOverloaded is re-asserted on every
	// tick, including while the signal stays missing
	ev.tick(t0.Add(cfg.Sustain + 2*cfg.Interval))
	assert.False(t, ev.IsOverloaded())
	assert.Equal(t, 2, evaluations)
}

func TestActiveRequests_PrefersTheHostCount(t *testing.T) {
	ev := newTestEvictor(DefaultEvictorConfig())

	// without an override the manager's own count is used
	assert.Equal(t, 3, ev.activeRequests(3))

	// the host counts requests still setting up, which the manager does not hold yet
	ev.CountActiveRequestsWith(func() int { return 7 })
	assert.Equal(t, 7, ev.activeRequests(3))
}
