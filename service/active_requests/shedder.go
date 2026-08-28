package active_requests

import (
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/substreams/metrics"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type SheddingMode string

const (
	SheddingOff     SheddingMode = "off"
	SheddingObserve SheddingMode = "observe"  // evaluate and log, never cancel
	SheddingDevOnly SheddingMode = "dev-only" // only dev-mode requests may be cancelled
	SheddingFull    SheddingMode = "full"     // production requests may be cancelled too
)

func ParseSheddingMode(s string) (SheddingMode, error) {
	switch SheddingMode(s) {
	case SheddingOff, SheddingObserve, SheddingDevOnly, SheddingFull:
		return SheddingMode(s), nil
	}
	return "", fmt.Errorf("invalid shedding mode %q (accepted: off, observe, dev-only, full)", s)
}

type ShedderConfig struct {
	Mode SheddingMode

	TargetRatio      float64 // batch shedding cuts until the projected CPU usage fits under this fraction of quota
	SoftThreshold    float64 // usage ratio above which the pod is mildly overloaded
	HardThreshold    float64 // usage ratio above which the pod is clearly overloaded (with ThrottleGate or PressureGate)
	ThrottleGate     float64 // throttle ratio confirming clear overload
	PressureGate     float64 // PSI some avg10 confirming clear overload
	RecoverThreshold float64 // usage ratio under which the pod recovers

	SoftSustain    time.Duration // mild condition must hold this long before triggering
	HardSustain    time.Duration // clear condition must hold this long before triggering
	RecoverSustain time.Duration // recovery condition must hold this long before the pod is ready again
	Interval       time.Duration // evaluation tick
	Cooldown       time.Duration // minimum delay between two shedding events
	DrainDelay     time.Duration // delay between going unready and the first cancellation, covering the LB routing lag

	MinAge          time.Duration // requests younger than this are never cancelled
	MinBurnCores    float64       // requests consuming less CPU than this are never cancelled (cutting them would not help)
	MaxProdFraction float64       // maximum fraction of production requests cancelled in a single event
}

func DefaultShedderConfig() ShedderConfig {
	return ShedderConfig{
		Mode:             SheddingOff,
		TargetRatio:      0.75,
		SoftThreshold:    0.85,
		HardThreshold:    0.95,
		ThrottleGate:     0.25,
		PressureGate:     0.25,
		RecoverThreshold: 0.75,
		SoftSustain:      20 * time.Second,
		HardSustain:      10 * time.Second,
		RecoverSustain:   30 * time.Second,
		Interval:         5 * time.Second,
		Cooldown:         15 * time.Second,
		DrainDelay:       8 * time.Second,
		MinAge:           90 * time.Second,
		MinBurnCores:     0.05,
		MaxProdFraction:  0.5,
	}
}

type shedClass int

const (
	classDev shedClass = iota // dev-mode requests, cancelled first
	classProdCatchup          // production-mode requests still catching up from files
	classProdLive             // production-mode requests streaming live blocks, cancelled last
)

func (c shedClass) String() string {
	switch c {
	case classDev:
		return "dev"
	case classProdCatchup:
		return "prod-catchup"
	case classProdLive:
		return "prod-live"
	}
	return "unknown"
}

type overloadLevel int

const (
	levelOK overloadLevel = iota
	levelMild
	levelClear
)

// Shedder detects CPU overload of the tier1 pod from its cgroup and cancels
// the most expensive / least important requests so their clients reconnect
// through the load balancer to a less busy pod. It flips the pod unready
// (IsOverloaded, checked by the health check) before cancelling anything and
// keeps it unready until CPU recovers.
type Shedder struct {
	cfg     ShedderConfig
	manager *ActiveRequestsManager
	reader  *CPUReader
	logger  *zap.Logger

	overloaded       atomic.Bool
	onOverloadChange func(overloaded bool)

	// zero time = condition not currently holding
	softSince    time.Time
	hardSince    time.Time
	recoverSince time.Time
	unreadyAt    time.Time
	lastShedAt   time.Time

	// uniqueID -> cumulative wasm compute at the previous tick
	prevCompute map[string]time.Duration
}

func NewShedder(cfg ShedderConfig, manager *ActiveRequestsManager, reader *CPUReader, logger *zap.Logger) *Shedder {
	return &Shedder{
		cfg:         cfg,
		manager:     manager,
		reader:      reader,
		logger:      logger,
		prevCompute: make(map[string]time.Duration),
	}
}

// IsOverloaded reports whether the pod is CPU-overloaded; the tier1 health
// check and admission path treat this like the active-requests soft limit.
func (sh *Shedder) IsOverloaded() bool {
	return sh.overloaded.Load()
}

// OnOverloadChange registers a callback fired on every overloaded-state edge,
// from the shedder's own goroutine. Must be called before Run.
func (sh *Shedder) OnOverloadChange(fn func(overloaded bool)) {
	sh.onOverloadChange = fn
}

// Run evaluates the shedding policy every Interval until stop is closed.
func (sh *Shedder) Run(stop <-chan struct{}) {
	sh.logger.Info("CPU shedder starting",
		zap.String("mode", string(sh.cfg.Mode)),
		zap.Float64("quota_cores", sh.reader.QuotaCores()),
	)
	if metrics.Tier1CPUQuotaCores != nil {
		metrics.Tier1CPUQuotaCores.SetFloat64(sh.reader.QuotaCores())
	}
	ticker := time.NewTicker(sh.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			sh.tick(time.Now())
		}
	}
}

func (sh *Shedder) tick(now time.Time) {
	signals, err := sh.reader.Read()
	if err != nil {
		sh.logger.Warn("cannot read cgroup CPU signals", zap.Error(err))
		return
	}
	sh.publishMetrics(signals)

	candidates, totalProd := sh.sampleBurnRates(now)
	level := sh.classify(signals, now)

	if !sh.overloaded.Load() {
		return
	}
	if now.Sub(sh.unreadyAt) < sh.cfg.DrainDelay {
		return
	}
	if !sh.lastShedAt.IsZero() && now.Sub(sh.lastShedAt) < sh.cfg.Cooldown {
		return
	}
	if level == levelOK {
		// overloaded (not yet recovered) but no condition currently firing: hold, don't shed
		return
	}

	if sh.cfg.Mode == SheddingDevOnly {
		onlyDev := candidates[:0]
		for _, c := range candidates {
			if c.class == classDev {
				onlyDev = append(onlyDev, c)
			}
		}
		candidates = onlyDev
	}

	usedCores := signals.UsageRatio * signals.QuotaCores
	targetCores := sh.cfg.TargetRatio * signals.QuotaCores
	excessCores := usedCores - targetCores

	victims := selectVictims(candidates, excessCores, level == levelClear, sh.cfg.MaxProdFraction, totalProd)
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
			zap.Float64("cpu_throttle_ratio", signals.ThrottleRatio),
			zap.String("level", map[overloadLevel]string{levelMild: "mild", levelClear: "clear"}[level]),
		}
		if sh.cfg.Mode == SheddingObserve {
			sh.logger.Info("CPU overloaded: would cancel request (observe mode)", fields...)
			recordShed(v.class, "observed")
			continue
		}
		sh.logger.Warn("CPU overloaded: cancelling request", fields...)
		recordShed(v.class, "cancelled")
		v.cancel(connect.NewError(connect.CodeUnavailable, fmt.Errorf("server CPU overloaded, please reconnect")))
	}
	sh.lastShedAt = now
}

// classify updates the sustain windows and the overloaded flag, and returns
// the overload level currently firing.
func (sh *Shedder) classify(signals CPUSignals, now time.Time) overloadLevel {
	softCond := signals.UsageRatio >= sh.cfg.SoftThreshold
	hardCond := signals.UsageRatio >= sh.cfg.HardThreshold &&
		(signals.ThrottleRatio >= sh.cfg.ThrottleGate || signals.PressureAvg10 >= sh.cfg.PressureGate)
	recoverCond := signals.UsageRatio < sh.cfg.RecoverThreshold

	sh.softSince = holdSince(sh.softSince, softCond, now)
	sh.hardSince = holdSince(sh.hardSince, hardCond, now)
	sh.recoverSince = holdSince(sh.recoverSince, recoverCond, now)

	level := levelOK
	if !sh.softSince.IsZero() && now.Sub(sh.softSince) >= sh.cfg.SoftSustain {
		level = levelMild
	}
	if !sh.hardSince.IsZero() && now.Sub(sh.hardSince) >= sh.cfg.HardSustain {
		level = levelClear
	}

	if level != levelOK && !sh.overloaded.Load() {
		sh.overloaded.Store(true)
		sh.unreadyAt = now
		sh.logger.Warn("pod is CPU-overloaded, advertising unready",
			zap.Float64("cpu_usage_ratio", signals.UsageRatio),
			zap.Float64("cpu_throttle_ratio", signals.ThrottleRatio),
			zap.Float64("cpu_pressure_some_avg10", signals.PressureAvg10),
		)
		if sh.onOverloadChange != nil {
			sh.onOverloadChange(true)
		}
	}
	if sh.overloaded.Load() && !sh.recoverSince.IsZero() && now.Sub(sh.recoverSince) >= sh.cfg.RecoverSustain {
		sh.overloaded.Store(false)
		sh.unreadyAt = time.Time{}
		sh.logger.Info("pod CPU recovered, advertising ready again",
			zap.Float64("cpu_usage_ratio", signals.UsageRatio),
		)
		if sh.onOverloadChange != nil {
			sh.onOverloadChange(false)
		}
	}
	return level
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

type shedCandidate struct {
	uniqueID     string
	traceID      string
	class        shedClass
	burnCores    float64
	startTime    time.Time
	currentBlock uint64
	cancel       func(error)
}

// sampleBurnRates computes each request's CPU burn rate (in cores) since the
// previous tick and returns the shedding candidates: old enough, burning
// enough to be worth cancelling. It also refreshes BurnCores on the records
// for the debug API listing.
func (sh *Shedder) sampleBurnRates(now time.Time) (candidates []shedCandidate, totalProd int) {
	sh.manager.Lock()
	defer sh.manager.Unlock()

	seen := make(map[string]bool, len(sh.manager.reqs))
	for uniqueID, req := range sh.manager.reqs {
		seen[uniqueID] = true
		if req.ProductionMode {
			totalProd++
		}
		if req.stats == nil {
			continue
		}
		compute := req.stats.LocalWasmComputeDuration()
		prev, sampled := sh.prevCompute[uniqueID]
		sh.prevCompute[uniqueID] = compute

		var burn float64
		if sampled && compute > prev {
			burn = float64(compute-prev) / float64(sh.cfg.Interval)
		}
		req.BurnCores = burn

		if !sampled || now.Sub(req.StartTime) < sh.cfg.MinAge || burn < sh.cfg.MinBurnCores {
			continue
		}
		class := classDev
		if req.ProductionMode {
			class = classProdCatchup
			if req.Live {
				class = classProdLive
			}
		}
		candidates = append(candidates, shedCandidate{
			uniqueID:     uniqueID,
			traceID:      req.TraceID,
			class:        class,
			burnCores:    burn,
			startTime:    req.StartTime,
			currentBlock: req.stats.CurrentBlock(),
			cancel:       req.cancelFunc,
		})
	}
	for uniqueID := range sh.prevCompute {
		if !seen[uniqueID] {
			delete(sh.prevCompute, uniqueID)
		}
	}
	return candidates, totalProd
}

// selectVictims picks which candidates to cancel: least important class first,
// highest burn first within a class. In batch mode (clear overload) it keeps
// cutting until the cancelled burn covers excessCores; otherwise it returns a
// single victim. At most maxProdFraction of the totalProd production requests
// are cut in one event, which also protects a lone production request from
// ever being cancelled.
func selectVictims(candidates []shedCandidate, excessCores float64, batch bool, maxProdFraction float64, totalProd int) []shedCandidate {
	sorted := make([]shedCandidate, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].class != sorted[j].class {
			return sorted[i].class < sorted[j].class
		}
		return sorted[i].burnCores > sorted[j].burnCores
	})

	maxProd := int(maxProdFraction * float64(totalProd))
	var out []shedCandidate
	var shedCores float64
	var shedProd int
	for _, c := range sorted {
		if c.class != classDev {
			if shedProd >= maxProd {
				continue
			}
			shedProd++
		}
		out = append(out, c)
		shedCores += c.burnCores
		if !batch || shedCores >= excessCores {
			break
		}
	}
	return out
}

func (sh *Shedder) publishMetrics(signals CPUSignals) {
	if metrics.Tier1CPUUsageRatio == nil {
		return
	}
	metrics.Tier1CPUUsageRatio.SetFloat64(signals.UsageRatio)
	metrics.Tier1CPUThrottleRatio.SetFloat64(signals.ThrottleRatio)
	metrics.Tier1CPUPressureSomeAvg10.SetFloat64(signals.PressureAvg10)
	if sh.overloaded.Load() {
		metrics.Tier1CPUOverloaded.SetUint64(1)
	} else {
		metrics.Tier1CPUOverloaded.SetUint64(0)
	}
}

func recordShed(class shedClass, action string) {
	if metrics.Tier1ShedRequestsCounter == nil {
		return
	}
	metrics.Tier1ShedRequestsCounter.Inc(class.String(), action)
}
