# CPU-based request shedding on tier1

## Problem

Tier1 pods serve long-lived gRPC streams (sometimes 24h+) behind a GCP load
balancer. Request-count limits (soft=advertise-unready, hard=refuse) balance
the *number* of streams, but a few CPU-heavy requests can land on the same pod
and saturate its CPU quota. Every stream on that pod then lags. The hub does
not lag, so `head_block_drift` stays flat and nothing detects the condition.
Rollouts make it worse: reconnecting streams all do catchup at once (worst-case
CPU profile) on the fewest ready pods.

## Approach

The pod detects its own CPU overload from its cgroup, attributes CPU to
individual requests using the per-request wasm execution timer that already
exists, and cancels the most expensive / least important requests with
`CodeUnavailable` so clients reconnect through the LB to a less busy pod.
While overloaded, the pod also advertises unready and refuses new requests,
and it always drains from the LB *before* cancelling so shed clients don't
reconnect into the same pod.

Priority order for cancellation (least important first):

1. dev-mode requests, highest CPU burn first
2. production-mode requests not yet live (catchup/backfill), highest burn first
3. production-mode live requests, highest burn first (capped per event)

Two aggressiveness tiers:

- **Clear overload** (cpuRatio ≥ hard-threshold AND throttled/PSI, short
  sustain): go unready, wait drain-delay, then cancel a *batch* in one shot —
  cut victims in priority order until the summed burn rate of survivors fits
  under target × quota. Rip the bandaid; don't make customers lag for minutes
  while we shed one victim at a time.
- **Mild overload** (cpuRatio ≥ soft-threshold, longer sustain): go unready,
  shed the single top victim per cooldown.

## Signals

All read from the container's own cgroup v2 (per-container, unaffected by
neighbor pods on the same GKE node):

- `cpu.stat`: `usage_usec` (usage vs quota), `nr_throttled`/`throttled_usec`
  (we are being capped at our own limit)
- `cpu.pressure`: `some avg10` (PSI — our runnable threads waited for CPU,
  catches neighbor contention too)
- `cpu.max`: quota (cores)

Per-request CPU burn: `pipeline/exec/module_executor.go` already wraps every
module execution in `RecordModuleWasmBlockBegin/End` on the request's
`metrics.Stats`, accumulating wall time into `processingTime`. Wasm does not
block on network (except extension calls, tracked separately and subtracted),
so wall ≈ CPU for the compute-bound requests we care about. A sampler diffs
the cumulative total every tick into a burn rate in cores — no new hot-path
instrumentation. Validation: observe mode + a pprof CPU profile with
trace_id goroutine labels on one hot pod, compare rankings.

## Implementation

### 1. CPU signal reader (`service/active_requests/cpu.go`)

Cgroup v2 reader for the values above (same package already reads cgroup
memory via `memlimit.FromCgroup()`). New dmetrics gauges: cpu ratio, throttle
ratio, PSI some avg10 — observable in Prometheus before anything enforces.

### 2. Per-request accounting

- `activeRequestRecord` gains `ProductionMode`, `IsLive`, and a reference to
  the request's `metrics.Stats`.
- `Stats` gains an accessor returning total module `processingTime` minus
  external-call time, and the current block.
- Pipeline flips `IsLive` on the handler at the file-source → hub transition.
- `tier1.go` passes the new fields at `activeRequestsManager.Add()`.
- Sampler in the manager computes burn rate per tick.

### 3. Shedder loop

Goroutine owned by `ActiveRequestsManager`. Each tick: read signals, classify
(ok / mild / clear), apply the tier policies above. Rules:

- always: unready first, wait `drain-delay`, then cancel
- skip requests younger than `min-age` (startup/store-load bursts look hot)
- at most `max-prod-shed-fraction` of production requests per event
- cancel cause: `CodeUnavailable`, "server overloaded, please reconnect"
- counter metric for shed events labeled by class + structured log per victim
  (trace_id, burn, class, block distance)

### 4. Readiness + admission

`getOverloadedStatus()` ORs the CPU-overload state so `canAcceptUpcomingRequests()`
(health check → LB) and the hard-limit refusal path both react to CPU pressure,
not just request count. Ready again when cpuRatio < `recover-threshold` for one
sustain window.

### 5. Config

`mode` knob: `off` | `observe` | `dev-only` | `full` (default `off`).
Tunables (defaults):

| flag | default | meaning |
|---|---|---|
| cpu-shedding-mode | off | off / observe / dev-only / full |
| cpu-shedding-target-ratio | 0.75 | post-shed CPU target (× quota) |
| cpu-shedding-soft-threshold | 0.85 | mild overload trigger |
| cpu-shedding-hard-threshold | 0.95 | clear overload trigger (with throttle/PSI) |
| cpu-shedding-soft-sustain | 20s | mild trigger must hold this long |
| cpu-shedding-hard-sustain | 10s | clear trigger must hold this long |
| cpu-shedding-interval | 5s | evaluation tick |
| cpu-shedding-cooldown | 15s | wait after a shed before re-evaluating |
| cpu-shedding-drain-delay | 8s | unready → first cancel (LB drain lag) |
| cpu-shedding-min-age | 90s | never cancel requests younger than this |
| cpu-shedding-max-prod-fraction | 0.5 | max share of prod requests cut per event |
| cpu-shedding-recover-threshold | 0.75 | below this → ready again |

Config lands on `app/tier1.go` Config; `--substreams-tier1-...` flag
registration lives in firehose-core (companion PR).

## Deployment changes (HPA / k8s)

- **HPA**: add a CPU utilization target (~70%) alongside the existing
  requests-per-pod target (14). Today one hot pod barely moves fleet-average
  CPU, so a CPU HPA would sleep through the incident; once shedding spreads
  heavy requests across pods, fleet-average CPU becomes a meaningful scaling
  signal and the two mechanisms reinforce each other: shedding fixes
  distribution, the CPU target adds capacity. With `autoscaling/v2`, keep both
  metrics in one HPA (it scales on whichever demands more replicas). Consider
  `behavior.scaleDown` stabilization (~5–10 min) so post-shed dips don't flap
  the fleet.
- **Rollout pile-up**: enable slow-start on the GCP backend service (or ramp
  the internal soft limit for the first few minutes after boot) so freshly
  ready pods don't absorb the whole reconnect wave in catchup mode.
- **Alerting**: alert on the new shed-event counter (any `full`-mode prod shed
  is worth eyes) and on sustained throttle ratio.

## Rollout

1. Deploy in `observe`: gauges + "would have cancelled" logs.
2. pprof trace_id-label comparison on one hot pod to validate attribution.
3. `dev-only`, watch for a week.
4. `full`, with HPA CPU target in place first (shedding without added
   capacity just moves the pain around).
