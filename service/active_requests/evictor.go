package active_requests

import (
	"fmt"
	"math"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/substreams/metrics"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type EvictionMode string

const (
	EvictionOff     EvictionMode = "off"
	EvictionObserve EvictionMode = "observe"  // evaluate and log, never cancel
	EvictionDevOnly EvictionMode = "dev-only" // only dev-mode requests may be cancelled
	EvictionFull    EvictionMode = "full"     // production requests may be cancelled too
)

func ParseEvictionMode(s string) (EvictionMode, error) {
	switch EvictionMode(s) {
	case EvictionOff, EvictionObserve, EvictionDevOnly, EvictionFull:
		return EvictionMode(s), nil
	}
	return "", fmt.Errorf("invalid eviction mode %q (accepted: off, observe, dev-only, full)", s)
}

type EvictorConfig struct {
	Mode EvictionMode

	Threshold        float64 // usage ratio above which the pod is CPU-overloaded
	RecoverThreshold float64 // usage ratio under which the pod recovers
	TargetRatio      float64 // eviction cuts until the projected CPU usage fits under this fraction of quota

	Sustain        time.Duration // the overload condition must hold this long before triggering
	RecoverSustain time.Duration // recovery condition must hold this long before the pod is ready again
	Interval       time.Duration // evaluation tick
	Cooldown       time.Duration // minimum delay between two eviction events
	DrainDelay     time.Duration // delay between going unready and the first cancellation, covering the LB routing lag

	MinAge       time.Duration // requests younger than this are never cancelled
	MinBurnCores float64       // requests consuming less CPU than this are never cancelled (cutting them would not help)

	// NominalCapacity is how many requests a pod carries when it is full, used
	// only to scale substreams_tier1_effective_active_requests. Set it to the
	// per-pod request target of the horizontal autoscaler, so that a pod held
	// at TargetRatio reports exactly that target. Zero disables the adjustment
	// and the metric reports the plain active-request count.
	NominalCapacity float64
}

// WithDefaults returns the config with every unset (zero) tunable replaced by
// its default, so operators only need to set the mode and the values they want
// to override.
func (c EvictorConfig) WithDefaults() EvictorConfig {
	def := DefaultEvictorConfig()
	if c.Mode == "" {
		c.Mode = EvictionOff
	}
	setFloat := func(v *float64, d float64) {
		if *v == 0 {
			*v = d
		}
	}
	setDuration := func(v *time.Duration, d time.Duration) {
		if *v == 0 {
			*v = d
		}
	}
	setFloat(&c.Threshold, def.Threshold)
	setFloat(&c.RecoverThreshold, def.RecoverThreshold)
	setFloat(&c.TargetRatio, def.TargetRatio)
	setDuration(&c.Sustain, def.Sustain)
	setDuration(&c.RecoverSustain, def.RecoverSustain)
	setDuration(&c.Interval, def.Interval)
	setDuration(&c.Cooldown, def.Cooldown)
	setDuration(&c.DrainDelay, def.DrainDelay)
	setDuration(&c.MinAge, def.MinAge)
	setFloat(&c.MinBurnCores, def.MinBurnCores)
	return c
}

func DefaultEvictorConfig() EvictorConfig {
	return EvictorConfig{
		Mode:             EvictionOff,
		Threshold:        0.90,
		RecoverThreshold: 0.75,
		TargetRatio:      0.75,
		Sustain:          15 * time.Second,
		RecoverSustain:   30 * time.Second,
		Interval:         5 * time.Second,
		Cooldown:         15 * time.Second,
		DrainDelay:       8 * time.Second,
		MinAge:           90 * time.Second,
		MinBurnCores:     0.05,
	}
}

type evictClass int

// Eviction order. A dev-mode request is a developer iterating and is always
// cut first. Between the two production classes, a live request burning a lot
// of CPU keeps burning it for as long as it stays connected, while a catching
// up request is spending CPU on work it will finish and then stop needing;
// cutting the catchup one throws away progress that has to be redone.
const (
	classDev         evictClass = iota // dev-mode requests, cancelled first
	classProdLive                      // production-mode requests streaming live blocks
	classProdCatchup                   // production-mode requests still catching up from files, cancelled last
)

func (c evictClass) String() string {
	switch c {
	case classDev:
		return "dev"
	case classProdLive:
		return "prod-live"
	case classProdCatchup:
		return "prod-catchup"
	}
	return "unknown"
}

// Evictor detects CPU overload of the tier1 pod from its cgroup and cancels
// the most expensive / least important requests so their clients reconnect
// through the load balancer to a less busy pod. It flips the pod unready
// (IsOverloaded, checked by the health check) before cancelling anything and
// keeps it unready until CPU recovers.
type Evictor struct {
	cfg     EvictorConfig
	manager *ActiveRequestsManager
	reader  *CPUReader
	logger  *zap.Logger

	overloaded       atomic.Bool
	onOverloadChange func(overloaded bool)

	// zero time = condition not currently holding
	overloadSince time.Time
	recoverSince  time.Time
	unreadyAt     time.Time
	lastEvictAt   time.Time

	// uniqueID -> cumulative wasm compute at the previous tick
	prevCompute map[string]time.Duration
}

func NewEvictor(cfg EvictorConfig, manager *ActiveRequestsManager, reader *CPUReader, logger *zap.Logger) *Evictor {
	return &Evictor{
		cfg:         cfg,
		manager:     manager,
		reader:      reader,
		logger:      logger,
		prevCompute: make(map[string]time.Duration),
	}
}

// enforcing reports whether the evictor is allowed to act (flip readiness,
// refuse admission, cancel requests); in observe mode it only watches and logs.
func (ev *Evictor) enforcing() bool {
	return ev.cfg.Mode == EvictionDevOnly || ev.cfg.Mode == EvictionFull
}

// IsOverloaded reports whether the pod is CPU-overloaded; the tier1 health
// check and admission path treat this like the active-requests soft limit.
// Always false in observe mode, which must not affect routing or admission.
func (ev *Evictor) IsOverloaded() bool {
	return ev.enforcing() && ev.overloaded.Load()
}

// OnOverloadChange registers a callback fired on every overloaded-state edge,
// from the evictor's own goroutine. Must be called before Run.
func (ev *Evictor) OnOverloadChange(fn func(overloaded bool)) {
	ev.onOverloadChange = fn
}

// Run evaluates the eviction policy every Interval until stop is closed.
func (ev *Evictor) Run(stop <-chan struct{}) {
	ev.logger.Info("CPU evictor starting",
		zap.String("mode", string(ev.cfg.Mode)),
		zap.Float64("quota_cores", ev.reader.QuotaCores()),
	)
	if metrics.Tier1CPUQuotaCores != nil {
		metrics.Tier1CPUQuotaCores.SetFloat64(ev.reader.QuotaCores())
	}
	ticker := time.NewTicker(ev.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ev.tick(time.Now())
		}
	}
}

func (ev *Evictor) tick(now time.Time) {
	signals, err := ev.reader.Read()
	if err != nil {
		ev.logger.Warn("cannot read cgroup CPU signals", zap.Error(err))
		return
	}
	candidates, totalActive := ev.sampleBurnRates(now)
	firing := ev.classify(signals, now)
	ev.publishMetrics(signals, totalActive)

	if !ev.overloaded.Load() {
		return
	}
	if now.Sub(ev.unreadyAt) < ev.cfg.DrainDelay {
		return
	}
	if !ev.lastEvictAt.IsZero() && now.Sub(ev.lastEvictAt) < ev.cfg.Cooldown {
		return
	}
	if !firing {
		// overloaded (not yet recovered) but usage is back under the threshold: hold, do not evict
		return
	}

	if ev.cfg.Mode == EvictionDevOnly {
		onlyDev := candidates[:0]
		for _, c := range candidates {
			if c.class == classDev {
				onlyDev = append(onlyDev, c)
			}
		}
		candidates = onlyDev
	}

	usedCores := signals.UsageRatio * signals.QuotaCores
	targetCores := ev.cfg.TargetRatio * signals.QuotaCores
	excessCores := usedCores - targetCores

	victims := selectVictims(candidates, excessCores)
	if len(victims) == 0 {
		return
	}

	for _, v := range victims {
		fields := []zap.Field{
			zap.String("trace_id", v.traceID),
			zap.String("class", v.class.String()),
			zap.Float64("burn_cores", v.burnCores),
			zap.Duration("age", now.Sub(v.startTime)),
			zap.Uint64("current_block", v.currentBlock),
			zap.Float64("cpu_usage_ratio", signals.UsageRatio),
		}
		if ev.cfg.Mode == EvictionObserve {
			ev.logger.Info("CPU overloaded: would cancel request (observe mode)", fields...)
			recordEviction(v.class, "observed")
			continue
		}
		ev.logger.Warn("CPU overloaded: cancelling request", fields...)
		recordEviction(v.class, "cancelled")
		v.cancel(connect.NewError(connect.CodeUnavailable, fmt.Errorf("server CPU overloaded, please reconnect")))
	}
	ev.lastEvictAt = now
}

// classify updates the sustain windows and the overloaded flag, and reports
// whether the overload condition is currently firing.
func (ev *Evictor) classify(signals CPUSignals, now time.Time) bool {
	ev.overloadSince = holdSince(ev.overloadSince, signals.UsageRatio >= ev.cfg.Threshold, now)
	ev.recoverSince = holdSince(ev.recoverSince, signals.UsageRatio < ev.cfg.RecoverThreshold, now)

	firing := !ev.overloadSince.IsZero() && now.Sub(ev.overloadSince) >= ev.cfg.Sustain

	if firing && !ev.overloaded.Load() {
		ev.overloaded.Store(true)
		ev.unreadyAt = now
		ev.logger.Warn("pod is CPU-overloaded",
			zap.Bool("enforced", ev.enforcing()),
			zap.Float64("cpu_usage_ratio", signals.UsageRatio),
		)
		if ev.onOverloadChange != nil && ev.enforcing() {
			ev.onOverloadChange(true)
		}
	}
	if ev.overloaded.Load() && !ev.recoverSince.IsZero() && now.Sub(ev.recoverSince) >= ev.cfg.RecoverSustain {
		ev.overloaded.Store(false)
		ev.unreadyAt = time.Time{}
		ev.logger.Info("pod CPU recovered",
			zap.Bool("enforced", ev.enforcing()),
			zap.Float64("cpu_usage_ratio", signals.UsageRatio),
		)
		if ev.onOverloadChange != nil && ev.enforcing() {
			ev.onOverloadChange(false)
		}
	}
	return firing
}

func holdSince(since time.Time, condition bool, now time.Time) time.Time {
	if !condition {
		return time.Time{}
	}
	if since.IsZero() {
		return now
	}
	return since
}

type evictCandidate struct {
	uniqueID     string
	traceID      string
	class        evictClass
	burnCores    float64
	startTime    time.Time
	currentBlock uint64
	cancel       func(error)
}

// sampleBurnRates computes each request's CPU burn rate (in cores) since the
// previous tick and returns the eviction candidates: old enough, burning
// enough to be worth cancelling. It also refreshes BurnCores on the records
// for the debug API listing.
func (ev *Evictor) sampleBurnRates(now time.Time) (candidates []evictCandidate, totalActive int) {
	ev.manager.Lock()
	defer ev.manager.Unlock()

	seen := make(map[string]bool, len(ev.manager.reqs))
	totalActive = len(ev.manager.reqs)
	for uniqueID, req := range ev.manager.reqs {
		seen[uniqueID] = true
		if req.stats == nil {
			continue
		}
		compute := req.stats.LocalWasmComputeDuration()
		prev, sampled := ev.prevCompute[uniqueID]
		ev.prevCompute[uniqueID] = compute

		var burn float64
		if sampled && compute > prev {
			burn = float64(compute-prev) / float64(ev.cfg.Interval)
		}
		req.BurnCores = burn

		if !sampled || now.Sub(req.StartTime) < ev.cfg.MinAge || burn < ev.cfg.MinBurnCores {
			continue
		}
		class := classDev
		if req.ProductionMode {
			class = classProdCatchup
			if req.Live {
				class = classProdLive
			}
		}
		candidates = append(candidates, evictCandidate{
			uniqueID:     uniqueID,
			traceID:      req.TraceID,
			class:        class,
			burnCores:    burn,
			startTime:    req.StartTime,
			currentBlock: req.stats.CurrentBlock(),
			cancel:       req.cancelFunc,
		})
	}
	for uniqueID := range ev.prevCompute {
		if !seen[uniqueID] {
			delete(ev.prevCompute, uniqueID)
		}
	}
	return candidates, totalActive
}

// selectVictims picks which candidates to cancel: least important class first,
// highest burn first within a class, cutting until the cancelled burn covers
// excessCores.
func selectVictims(candidates []evictCandidate, excessCores float64) []evictCandidate {
	sorted := make([]evictCandidate, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].class != sorted[j].class {
			return sorted[i].class < sorted[j].class
		}
		return sorted[i].burnCores > sorted[j].burnCores
	})

	var out []evictCandidate
	var evictedCores float64
	for _, c := range sorted {
		out = append(out, c)
		evictedCores += c.burnCores
		if evictedCores >= excessCores {
			break
		}
	}
	return out
}

// effectiveActiveRequests expresses the pod's load in request units, for the
// horizontal autoscaler: the plain active-request count, or the number of
// requests the CPU budget is actually being spent at, whichever is higher.
// Requests are not equally expensive, so a pod can be full at five of them; and
// since eviction holds CPU at TargetRatio, a eviction pod reports exactly
// NominalCapacity instead of looking idle once it has cancelled its way back
// under the threshold.
func (ev *Evictor) effectiveActiveRequests(signals CPUSignals, activeRequests int) float64 {
	if ev.cfg.NominalCapacity <= 0 || ev.cfg.TargetRatio <= 0 {
		return float64(activeRequests)
	}
	cpuEquivalentRequests := ev.cfg.NominalCapacity * (signals.UsageRatio / ev.cfg.TargetRatio)
	return math.Max(float64(activeRequests), cpuEquivalentRequests)
}

func (ev *Evictor) publishMetrics(signals CPUSignals, activeRequests int) {
	if metrics.Tier1CPUUsageRatio == nil {
		return
	}
	metrics.Tier1CPUUsageRatio.SetFloat64(signals.UsageRatio)
	metrics.Tier1EffectiveActiveRequests.SetFloat64(ev.effectiveActiveRequests(signals, activeRequests))
	if ev.overloaded.Load() {
		metrics.Tier1CPUOverloaded.SetUint64(1)
	} else {
		metrics.Tier1CPUOverloaded.SetUint64(0)
	}
}

func recordEviction(class evictClass, action string) {
	if metrics.Tier1EvictedRequestsCounter == nil {
		return
	}
	metrics.Tier1EvictedRequestsCounter.Inc(class.String(), action)
}
