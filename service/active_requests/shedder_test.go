package active_requests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSelectVictims_ClassThenBurnOrder(t *testing.T) {
	candidates := []shedCandidate{
		{traceID: "prod-live-big", class: classProdLive, burnCores: 3.0},
		{traceID: "dev-small", class: classDev, burnCores: 0.2},
		{traceID: "dev-big", class: classDev, burnCores: 1.5},
		{traceID: "catchup", class: classProdCatchup, burnCores: 2.0},
	}

	// single-victim mode: least important class wins even against a bigger burner
	victims := selectVictims(candidates, 10, false, 0.5, 4)
	require.Len(t, victims, 1)
	assert.Equal(t, "dev-big", victims[0].traceID)

	// batch mode: cut in class order until the shed burn covers the excess
	victims = selectVictims(candidates, 3.0, true, 0.5, 4)
	require.Len(t, victims, 3)
	assert.Equal(t, "dev-big", victims[0].traceID)
	assert.Equal(t, "dev-small", victims[1].traceID)
	assert.Equal(t, "catchup", victims[2].traceID)
}

func TestSelectVictims_ProdFractionCap(t *testing.T) {
	candidates := []shedCandidate{
		{traceID: "catchup-1", class: classProdCatchup, burnCores: 2.0},
		{traceID: "catchup-2", class: classProdCatchup, burnCores: 1.8},
		{traceID: "live-1", class: classProdLive, burnCores: 1.5},
	}

	// 4 production requests total, cap 0.5 => at most 2 cut, even with excess left to cover
	victims := selectVictims(candidates, 100, true, 0.5, 4)
	require.Len(t, victims, 2)
	assert.Equal(t, "catchup-1", victims[0].traceID)
	assert.Equal(t, "catchup-2", victims[1].traceID)

	// a lone production request is never cancelled: floor(0.5 * 1) = 0
	lone := []shedCandidate{{traceID: "lone", class: classProdCatchup, burnCores: 4.0}}
	assert.Empty(t, selectVictims(lone, 100, true, 0.5, 1))
}

func TestSelectVictims_SingleModeSkipsCappedProd(t *testing.T) {
	// dev-only filtering happens upstream; with no dev candidate and prod capped
	// out, nothing is selected
	candidates := []shedCandidate{{traceID: "live", class: classProdLive, burnCores: 2.0}}
	assert.Empty(t, selectVictims(candidates, 1, false, 0.5, 1))
}

func newTestShedder(cfg ShedderConfig) *Shedder {
	return NewShedder(cfg, nil, nil, zap.NewNop())
}

func TestClassify_SustainAndRecovery(t *testing.T) {
	cfg := DefaultShedderConfig()
	cfg.Mode = SheddingFull // IsOverloaded only reports in an enforcing mode
	sh := newTestShedder(cfg)
	t0 := time.Now()

	calm := CPUSignals{QuotaCores: 4, UsageRatio: 0.5}
	mild := CPUSignals{QuotaCores: 4, UsageRatio: 0.9}
	clear := CPUSignals{QuotaCores: 4, UsageRatio: 0.97, ThrottleRatio: 0.5}

	assert.Equal(t, levelOK, sh.classify(calm, t0))
	assert.False(t, sh.IsOverloaded())

	// mild condition fires only after SoftSustain
	assert.Equal(t, levelOK, sh.classify(mild, t0))
	assert.Equal(t, levelOK, sh.classify(mild, t0.Add(cfg.SoftSustain/2)))
	assert.False(t, sh.IsOverloaded())
	assert.Equal(t, levelMild, sh.classify(mild, t0.Add(cfg.SoftSustain)))
	assert.True(t, sh.IsOverloaded())

	// a dip resets the sustain window
	assert.Equal(t, levelOK, sh.classify(calm, t0.Add(cfg.SoftSustain+time.Second)))
	assert.Equal(t, levelOK, sh.classify(mild, t0.Add(cfg.SoftSustain+2*time.Second)))

	// still overloaded until the recovery condition holds RecoverSustain
	assert.True(t, sh.IsOverloaded())
	assert.Equal(t, levelOK, sh.classify(calm, t0.Add(time.Minute)))
	assert.True(t, sh.IsOverloaded())
	assert.Equal(t, levelOK, sh.classify(calm, t0.Add(time.Minute+cfg.RecoverSustain)))
	assert.False(t, sh.IsOverloaded())

	// clear condition has its own (shorter) sustain and needs throttle or PSI
	sh = newTestShedder(cfg)
	assert.Equal(t, levelOK, sh.classify(clear, t0))
	assert.Equal(t, levelClear, sh.classify(clear, t0.Add(cfg.HardSustain)))
	assert.True(t, sh.IsOverloaded())

	// 97% usage without throttling or pressure is not "clear", only building toward mild
	sh = newTestShedder(cfg)
	quiet97 := CPUSignals{QuotaCores: 4, UsageRatio: 0.97}
	assert.Equal(t, levelOK, sh.classify(quiet97, t0))
	assert.Equal(t, levelMild, sh.classify(quiet97, t0.Add(cfg.SoftSustain)))
}

func TestHoldSince(t *testing.T) {
	t0 := time.Now()
	assert.True(t, holdSince(time.Time{}, false, t0).IsZero())
	assert.Equal(t, t0, holdSince(time.Time{}, true, t0))
	assert.Equal(t, t0, holdSince(t0, true, t0.Add(time.Minute)))
	assert.True(t, holdSince(t0, false, t0.Add(time.Minute)).IsZero())
}

func TestShedderConfig_WithDefaults(t *testing.T) {
	def := DefaultShedderConfig()

	// zero config gets every tunable filled, mode stays off
	filled := ShedderConfig{}.WithDefaults()
	assert.Equal(t, SheddingOff, filled.Mode)
	filled.Mode = def.Mode
	assert.Equal(t, def, filled)

	// explicit values are kept
	custom := ShedderConfig{Mode: SheddingFull, TargetRatio: 0.6, Interval: time.Second}.WithDefaults()
	assert.Equal(t, SheddingFull, custom.Mode)
	assert.Equal(t, 0.6, custom.TargetRatio)
	assert.Equal(t, time.Second, custom.Interval)
	assert.Equal(t, def.SoftThreshold, custom.SoftThreshold)
}

func TestObserveModeDoesNotEnforce(t *testing.T) {
	cfg := DefaultShedderConfig()
	cfg.Mode = SheddingObserve
	sh := newTestShedder(cfg)
	t0 := time.Now()

	var callbackFired bool
	sh.OnOverloadChange(func(bool) { callbackFired = true })

	clear := CPUSignals{QuotaCores: 4, UsageRatio: 0.97, ThrottleRatio: 0.5}
	sh.classify(clear, t0)
	assert.Equal(t, levelClear, sh.classify(clear, t0.Add(cfg.HardSustain)))

	// internal state tracks the overload, but nothing visible to routing/admission
	assert.True(t, sh.overloaded.Load())
	assert.False(t, sh.IsOverloaded())
	assert.False(t, callbackFired)
}

func TestEffectiveActiveRequests(t *testing.T) {
	cfg := DefaultShedderConfig()
	cfg.NominalCapacity = 14
	cfg.TargetRatio = 0.75
	sh := newTestShedder(cfg)

	tests := []struct {
		name           string
		activeRequests int
		usageRatio     float64
		expect         float64
	}{
		{"idle pod reports nothing", 0, 0, 0},
		{"cheap requests report their own count", 5, 0.10, 5},
		{"expensive requests report what they cost", 5, 0.30, 5.6},
		{"pod held at the shedding target reports full", 5, 0.75, 14},
		{"pod over the target reports more than full", 8, 0.95, 17.733333333333334},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sh.effectiveActiveRequests(CPUSignals{QuotaCores: 4, UsageRatio: tt.usageRatio}, tt.activeRequests)
			assert.InDelta(t, tt.expect, got, 0.0001)
		})
	}
}

func TestEffectiveActiveRequests_NoNominalCapacity(t *testing.T) {
	cfg := DefaultShedderConfig()
	sh := newTestShedder(cfg)
	assert.Equal(t, 6.0, sh.effectiveActiveRequests(CPUSignals{QuotaCores: 4, UsageRatio: 0.9}, 6))
}
