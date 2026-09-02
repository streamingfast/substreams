# CPU-based request eviction on tier1

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
and it always drains from the LB *before* cancelling so evicted clients don't
reconnect into the same pod.

One overload state: `cpuRatio ≥ threshold` held for `sustain`. The pod goes
unready, waits `drain-delay` for the LB to stop sending it traffic, then
cancels a batch in one shot — victims in priority order until the summed burn
rate of the survivors fits under `target-ratio × quota`. Cutting one request
per cooldown was the alternative and it means customers lag for minutes while
the pod works its way down; cut once, deep enough, and be done.

Only requests burning real CPU are candidates: below `min-burn-cores`,
cancelling one frees nothing and costs a reconnect. Among those, priority
order for cancellation is:

1. dev-mode requests, highest CPU burn first
2. production live requests, highest burn first
3. production requests still catching up from files, highest burn first

Live before catchup is deliberate. A live request burning 2 cores keeps
burning 2 cores for as long as it stays connected — it is a permanent tax on
whichever pod holds it, and moving it is the only thing that helps. A catchup
request burning 2 cores is spending them on work that ends: it will reach the
head and go cheap on its own, and cancelling it throws away the segment
progress it has to redo elsewhere.

## Signals

One signal: **CPU usage as a fraction of quota**, from the container's own
cgroup v2 — the `cpu.stat` `usage_usec` delta between ticks, divided by
elapsed time and by the `cpu.max` quota in cores. Per-container, so neighbor
pods on the same GKE node do not move it.

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
memory via `memlimit.FromCgroup()`). New dmetrics gauges: quota cores, usage
ratio, overloaded flag — observable in Prometheus before anything enforces.

### 2. Per-request accounting

- `activeRequestRecord` gains `ProductionMode`, `IsLive`, and a reference to
  the request's `metrics.Stats`.
- `Stats` gains an accessor returning total module `processingTime` minus
  external-call time, and the current block.
- Pipeline flips `IsLive` on the handler at the file-source → hub transition.
- `tier1.go` passes the new fields at `activeRequestsManager.Add()`.
- Sampler in the manager computes burn rate per tick.

### 3. Evictor loop

Goroutine owned by `ActiveRequestsManager`. Each tick: read the usage ratio,
update the sustain window, and if the overload is firing, cut a batch. Rules:

- always: unready first, wait `drain-delay`, then cancel
- skip requests younger than `min-age` (startup/store-load bursts look hot)
- cancel cause: `CodeUnavailable`, "server overloaded, please reconnect"
- counter metric for eviction events labeled by class + structured log per victim
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
| cpu-eviction-mode | off | off / observe / dev-only / full |
| cpu-eviction-threshold | 0.90 | usage ratio that puts the pod in overload |
| cpu-eviction-sustain | 15s | trigger must hold this long |
| cpu-eviction-target-ratio | 0.75 | post-eviction CPU target (× quota), sizes the batch |
| cpu-eviction-interval | 5s | evaluation tick |
| cpu-eviction-cooldown | 15s | wait after an eviction before re-evaluating |
| cpu-eviction-drain-delay | 8s | unready → first cancel (LB drain lag) |
| cpu-eviction-min-age | 90s | never cancel requests younger than this |
| cpu-eviction-min-burn-cores | 0.05 | never cancel requests burning less than this |
| cpu-eviction-recover-threshold | 0.75 | below this → ready again |
| cpu-eviction-recover-sustain | 30s | recovery must hold this long |
| cpu-eviction-nominal-capacity | soft limit | requests a full pod carries, scales the autoscaler metric |

Config lands on `app/tier1.go` Config; `--substreams-tier1-...` flag
registration lives in firehose-core (companion PR).

## Autoscaling

`substreams_active_requests` stops working as the HPA input once eviction is
on, because it measures what the pod is *serving*, and eviction exists to
reduce that. The harder a pod evicts, the lower the fleet average, the fewer
replicas the HPA asks for: overload makes the autoscaler shrink the fleet.
Unready pods make it worse — for a `Pods`-type metric the HPA puts them in the
ignored set and assumes zero usage when scaling up, so an evicting pod actively
drags the average down while still counting as a replica.

CPU utilization is not a sufficient replacement either. The evictor is a
controller holding CPU at `target-ratio`, so an autoscaler watching CPU is
watching a regulated variable: it reads ~0.75 no matter how much demand is
being turned away, and it goes quiet entirely once eviction has done its job.

### The metric

`substreams_tier1_effective_active_requests`, published by the evictor every
tick:

    cpuEquivalentRequests   = nominalCapacity * (usageRatio / targetRatio)
    effectiveActiveRequests = max(activeRequests, cpuEquivalentRequests)

`nominalCapacity` should be set to the HPA's per-pod request target, so that a
pod sitting at `target-ratio` reports exactly "full". It defaults to the
active-requests soft limit, which is the only capacity figure the pod knows on
its own. With capacity 14 and target ratio 0.75:

| state | active | cpu ratio | reported |
|---|---|---|---|
| fresh pod | 0 | ~0 | 0 |
| 5 cheap requests | 5 | 0.30 | 5.6 |
| 5 heavy requests, post-eviction | 5 | 0.75 | 14 |
| spike before eviction fires | 8 | 0.95 | 17.7 |

Row 3 is the point: an evicting pod reports itself full while serving five
requests, with no eviction bookkeeping, no decay window and no double
counting when an evicted client reconnects elsewhere — eviction pins CPU at the
target by construction. Row 4 shows it does not clip: a pod past its target
reports over capacity and pulls scale-up harder. When demand genuinely leaves,
CPU drops and the metric drops with it, which is the behaviour we want.

### HPA config

- Scale on `substreams_tier1_effective_active_requests` as an External metric
  with `AverageValue` (`desiredReplicas = ceil(sum / target)`), target **12**.
  Targeting below the nominal capacity of 14 reserves ~2 slots per pod, so
  fleet headroom is `2 × replicas` rather than a fixed count — HPA has no way
  to express a fleet-wide additive constant.
- Keep the existing `substreams_active_requests` metric in the same
  `autoscaling/v2` HPA. It governs the normal regime and the HPA scales on
  whichever metric demands more replicas, so adding the new one can't regress
  current behaviour.
- `behavior.scaleUp`: fast (100% or +4 pods / 15s, no stabilization).
  `behavior.scaleDown`: 900s stabilization and small steps. Long-lived streams
  make every removed pod expensive, and post-eviction dips must not flap the fleet.
- Note the default 10% HPA tolerance: a target of 12 doesn't act until ~13.2.
- **Scale-down picks unready pods first.** The ReplicaSet controller's deletion
  ranking checks readiness (3rd) before `pod-deletion-cost` (4th), so a
  CPU-overloaded pod — the one holding the most expensive long-lived streams —
  is the preferred kill target during any scale-down. The long `scaleDown`
  stabilization window above is the mitigation; keeping overload episodes short
  (one batch clears the excess in one shot) is the other.

### Deployment changes (k8s)

- **Rollout pile-up**: enable slow-start on the GCP backend service (or ramp
  the internal soft limit for the first few minutes after boot) so freshly
  ready pods don't absorb the whole reconnect wave in catchup mode.
- **Alerting**: alert on the new eviction counter (any `full`-mode production
  eviction is worth eyes). Add one on `substreams_tier1_cpu_overloaded == 1`
  across a large share of pods for several minutes: that means the fleet is
  under-provisioned, not that one pod drew a bad request.
- **Blackout backstop (optional)**: if every pod goes unready at once, no
  in-pod metric sees the demand piling up at the LB. Cloud Monitoring's
  frontend `loadbalancing.googleapis.com|https|request_count` filtered on
  `response_code_class = 500` does — use the frontend series, not
  `backend_request_count`, which never sees requests that found no backend.
  It's minutes-delayed and silent whenever at least one pod is ready, so it's
  an emergency floor, not a scaling signal. Bounding how long a pod may stay
  unready is the cheaper fix for the same failure.

## Rollout

1. Deploy in `observe`: gauges + "would have cancelled" logs.
2. pprof trace_id-label comparison on one hot pod to validate attribution.
3. `dev-only`, watch for a week.
4. `full`, with the HPA switched to `effective_active_requests` first
   (eviction without added capacity just moves the pain around).


**What's on the branch:**

1. `plans/2026-08-28-cpu-request-cutoff.md` — the plan, including the autoscaling and deployment changes (`substreams_tier1_effective_active_requests` as the HPA input, scale-down stabilization, LB slow-start, alerting on the eviction counter).
2. **cgroup CPU reader** (`service/active_requests/cpu.go`) — CPU usage as a fraction of the container's own cgroup quota; exported as `substreams_tier1_cpu_*` gauges.
3. **Per-request accounting** — each active request records production mode, whether it reached live blocks (first `StepNew` from the hub), and its wasm compute time excluding external-call waits (`Stats.LocalWasmComputeDuration`), sampled into a per-request burn rate in cores (visible in the debug API listing).
4. **Evictor** (`service/active_requests/evictor.go`) — usage ≥90% of quota held for 15s cancels one batch, sized to bring usage back under 75%. Victim order: dev by burn desc → prod-live → prod-catchup, with a 90s min-age and a 0.05-core min-burn floor. It always flips unready first and waits the 8s drain delay so evicted clients don't reconnect into the same pod. Cancellation is `CodeUnavailable` so clients retry elsewhere.
5. **Readiness/admission** — while overloaded the pod is unready and refuses new `Blocks` requests; ready again after CPU stays under 75% for 30s.
6. **Config** — `Tier1Config.CPUEviction`: mode `off` (default) / `observe` / `dev-only` / `full`, every threshold tunable, unset values take defaults. `observe` logs would-be victims and publishes metrics but never touches routing (that had a bug initially; fixed and pinned by a test).

**Tests:** everything builds; `active_requests`, `metrics`, `service`, `app` suites pass. `./pipeline` shows a hanging `Test_resolveStartBlockNum` — I verified it hangs identically on plain `develop` (`go test ./pipeline/ -run Test_resolveStartBlockNum -count=3` stuck in `resolveStartBlockNum`, `pipeline/resolve.go:200`), so it's a pre-existing flake, not from this branch.

**Not done here:** flag registration (`--substreams-tier1-cpu-eviction-*`) is a firehose-core companion change, and the HPA/Tanka changes live in your deploy repo. The CHANGELOG entry is in. Suggested first rollout step per the plan: deploy with mode `observe` and watch the new gauges plus the "would cancel" logs.
