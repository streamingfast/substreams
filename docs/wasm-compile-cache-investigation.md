# WASM compilation cache for tier2 — investigation

Date: 2026-10-16
Scope: measurement + recommendation only. No production code changed.
Prototype: `wasm/bench/compile_bench_test.go` (compile-only benchmark, kept),
`wasm/bench/shared_module_concurrency_test.go` (race-detector smoke test for
sharing one compiled module across goroutines, kept).

## 0. Is this worth building?

The question that actually decides this isn't "does caching save compile
time" (§2 shows it obviously does) — it's whether that saving moves anything
anyone cares about. Two different things can be saved, and they have very
different payoffs:

- **Wall-clock of one reprocessing/backfill job.** tier2 parallelizes
  segments across many pods, so one job's wall-clock is set by its *longest*
  parallel branch, not the sum of all segment compile times. Shaving 2–23 ms
  off each segment (§2.3, non-empty case) only shortens the job if compile is
  a meaningful fraction of that critical-path segment's duration — and at
  2–23 ms against a segment that (per the external claim and typical
  real-block-execution time) runs hundreds of ms to seconds, it mostly isn't.
  **A cache is very unlikely to turn a 3-hour backfill into anything
  noticeably shorter.** The lever that actually shortens backfills is the
  in-flight empty-segment-skip reorder (§5) — it removes a whole segment's
  work, not a few milliseconds of it.
- **Aggregate fleet CPU / infra cost**, summed across every tier2 job the
  fleet runs per day. This is where a cache could plausibly matter: every
  avoided recompile is CPU-seconds not spent, fleet-wide, continuously — even
  if no single job feels faster. Whether that sum is "a few CPU-minutes a
  day" (noise) or "a meaningful slice of tier2's compute bill" is exactly the
  number this investigation could **not** get in a sandboxed environment with
  no access to production tier2 metrics.

The good news: answering that doesn't require building a cache first.
`pipeline.go:972` already computes `loadingModuleDuration` per pipeline run —
today it's thrown away (only feeds the dead `ModuleCache`, §1). Exporting it
as a metric (sum or histogram, tagged by package/binary hash) and letting it
run in production for a few days gives a real, measured
"aggregate compile-seconds paid today" number, for free, with zero new risk.
**That number is the actual go/no-go gate** — if it's small relative to
tier2's total fleet CPU, stop here; building A/B/C would be optimizing
something that isn't costing anything. Treat §7's phased plan as starting
with that measurement, not with code.

You separately raised that a tier2 job has "good chances" of being
rescheduled onto the same tier2 pod/process multiple times — if true, that's
the concrete mechanism that makes option A (in-process cache, §3) pay off
even though it never crosses a process boundary (§2.6): repeat jobs hitting
the same already-warm pod reuse its cache for free. This wasn't verified
against the real scheduler in this investigation (see Open Questions, §6) but
if confirmed it argues for trying A specifically (cheapest, lowest-risk, and
directly matches that locality pattern) before spending anything on B/C's
extra infrastructure.

## 1. Current state (confirmed in code)

- wasmtime is the default runtime (`wasm/registry.go:60`). `SUBSTREAMS_WASM_RUNTIME`
  can force `wazero` for everything; TinyGo binaries always use wazero
  (`wasm/registry.go:87`).
- `wasm/wasmtime/module.go:31`: every call to `newModule` creates a **brand-new**
  `wasmtime.Engine` (`wasmtime.NewEngineWithConfig(wasmtime.NewConfig())`), then
  `wasmtime.NewModule(engine, wasmCode)` (line 42) unconditionally compiles —
  no hash lookup, no reuse. Fuel metering is disabled (commented out). No cache
  config (`CacheConfigLoadDefault`/`CacheConfigLoad`, available in
  wasmtime-go v41's `Config`) is ever set.
- `wasm/wazero/module.go:43-51`: opens `wazero.NewCompilationCacheWithDir` against
  a fixed on-disk dir (`os.TempDir()/wazero`) on every call, and uses it — this
  **is** a real, persistent, cross-process compilation cache, but today it's only
  exercised for TinyGo binaries or when `SUBSTREAMS_WASM_RUNTIME=wazero` is forced.
- `pipeline/cache/module.go`'s `ModuleCache` is dead: `NewModuleCache` has zero
  call sites repo-wide; `p.moduleCache` is never assigned in `pipeline.New`
  (`pipeline/pipeline.go`); the one `.Set` call site (`pipeline.go:968-970`) is
  behind a nil-guard that's always false, and there is no `.Get` call anywhere.
  It should be deleted, not revived — reviving it as-is wouldn't help: it caches
  per pipeline-run only and is keyed oddly (see below).
- `pipeline/pipeline.go:946-967` (`BuildModuleExecutors`): dedups compiles
  **within one request** via `loadedModules map[moduleKey]wasm.Module`, keyed
  by `(binaryIndex, binaryType)` — not a content hash. This map dies with the
  pipeline/request.
- `service/tier2.go:373-382` (`getWASMRegistry`) builds a fresh `Registry` (and
  therefore a fresh `Engine`) on every `ProcessRange` gRPC request
  (`service/tier2.go:408`). tier2 is a long-lived process handling many
  sequential/concurrent requests, so **every segment job recompiles every
  distinct binary from zero**, with no cache surviving across requests, let
  alone across processes/pods.
- Empty-segment skip (`service/tier2.go:601-627`) runs **after**
  `BuildModuleExecutors` (`tier2.go:597`) — compile cost is currently paid even
  for segments that turn out to need no execution. (Another session is moving
  this skip earlier; not duplicated here — see §5.)

## 2. Measurements

Machine: Apple M4 Max (arm64), `go test -bench`, wasmtime-go v41.0.0.
Binaries used: repo's own `wasm/bench/substreams_wasm/substreams.wasm` (118,708
bytes) and the two largest real `.spkg`-embedded Rust binaries in the repo's test
fixtures: `big_substreams` (29,706 bytes) and `wasmbindgen_substreams`
(345,496 bytes — the largest available locally; no multi-MB real-world package
was available in this environment to test an even larger case).

### 2.1 Compile time (steady state, `wasm/bench/compile_bench_test.go`, `-benchtime 10x`)

| binary | bytes | wasmtime | wazero (warm disk cache) |
|---|---|---|---|
| bench substreams.wasm | 118,708 | 8.7 ms | 1.2 ms |
| big_substreams | 29,706 | 2.2 ms | 0.4 ms |
| wasmbindgen_substreams | 345,496 | 11.5 ms | 2.2 ms |

wazero looking faster here is an artifact of its on-disk cache already being
warm from prior iterations in the same run (see 2.2) — it is not a faster
compiler in the cold case.

### 2.2 Cold vs. warm compile, separate OS processes (proves the caching effect)

Deleted `wazero`'s on-disk cache dir, then ran single-shot (`-benchtime 1x`)
compiles of `wasmbindgen_substreams` (345,496 bytes) as **separate process
invocations**:

| runtime | cache state | time |
|---|---|---|
| wazero | cold (first process, empty dir) | 44.2 ms |
| wazero | warm (2nd process onward, same dir) | 3.3–3.5 ms (~13x faster) |
| wasmtime | every process (no cache exists) | 19.7 / 23.0 / 20.8 ms, every time |

This is the core finding: wazero's existing `wazero.NewCompilationCacheWithDir`
already demonstrates, in this repo, that a persistent compile cache turns a
~20–44 ms cost into a ~3 ms cost for a 345 KB real module — and it survives
process restarts because it's on disk. wasmtime has no equivalent and pays the
full cost on literally every call, forever.

wazero is used here only as existing, in-repo proof that the caching
*mechanism* works — **not** as a proposed solution. wazero is not used in
production; every option in §3 targets wasmtime (`Module.Serialize` /
`NewModuleDeserialize*`, §2.6), not a runtime switch.

### 2.3 Share of a tier2 job

Each `.spkg` tested compiles to exactly **one** distinct binary per package
(all modules in a package share one Rust binary) — so per-job compile overhead
is one compile, not one-per-module, for typical packages (two if a
wasm-bindgen-shim variant is also loaded).

- **Non-empty segment** (1000 blocks of real execution): 2–23 ms of compile
  against a job that (per the external claim in the brief, 0.15–0.4 s/chunk,
  and consistent with typical tier2 segment durations) runs for hundreds of
  ms to low seconds — compile is a small single-digit-percent tax.
- **Empty/filtered segment**: today, compile still runs before the skip check
  (`tier2.go:597` vs. `601-627`), so for a segment that does no block
  execution at all, the 2–23 ms compile **is** effectively the entire cost of
  the job. This is the case a compile cache would help most, independent of
  size — and it's also exactly the case the in-flight empty-segment-skip
  reordering (§5) removes entirely once merged. The two changes are
  complementary, not overlapping: the reorder removes compile cost for segments
  that get skipped; a compile cache removes it for segments that don't.

We did not have access to live tier2 production traces in this sandboxed
environment to get an exact empty-vs-non-empty segment ratio or a hard
job-duration distribution — recommend pulling `loadingModuleDuration`
(`pipeline.go:972`, already computed, currently only used to populate the dead
`ModuleCache`) from real tier2 metrics before sizing this further.

### 2.4 Memory footprint (`syscall.Rusage.Maxrss` delta, 200 kept modules, arm64/macOS)

| configuration | wasm bytes | RSS per cached module |
|---|---|---|
| fresh `Engine` per module (today's code path, if modules were kept alive) | 118,708 | **4,181 KB** |
| one shared `Engine`, 200 `Module`s | 118,708 | **590 KB** (~5.0x source size) |
| one shared `Engine`, 200 `Module`s | 345,496 | **1,538 KB** (~4.5x source size) |
| wazero equivalent (Go-heap allocs only, not comparable 1:1 — native code is off-heap for wasmtime) | 345,496 | 1,340 KB heap allocs (compile-time only, not retained) |

Two things fall out of this:

1. **A fresh `Engine` per cached `Module` is ~7x more memory than it needs to
   be.** wasmtime's `Engine` carries substantial fixed overhead (JIT/codegen
   state) independent of the module compiled. Any cache design must share one
   long-lived `Engine` across all cached `Module`s, never create one per
   binary.
2. With a shared `Engine`, expect **roughly 4.5–5x the source `.wasm` size**
   per cached compiled module. For sizing an LRU: 100 cached modules averaging
   ~150 KB source ≈ 70 MB RSS; a fleet with many distinct packages needs this
   budgeted per tier2 pod, not assumed negligible.

### 2.5 Thread-safety of sharing Engine/Module across goroutines

wasmtime-go v41 does **not** document, anywhere in `engine.go`, `module.go`,
`store.go`, or `linker.go`, whether an `Engine` or compiled `Module` is safe to
share across `Store`s/goroutines (the only explicit thread-safety comment in
the binding is on `Engine.IncrementEpoch`). `Module.Serialize` /
`NewModuleDeserialize` / `NewModuleDeserializeFile` exist (`module.go:227`,
`182`, `206`) — deserialized bytes must come from "the same version of this
library" and matching engine config (undocumented further).

`wasm/bench/shared_module_concurrency_test.go` compiles one module once, then
runs 50 goroutines concurrently creating their own `Store`/`Linker`/instance
from that single shared `Engine`+`Module` and executing a real entrypoint —
passes clean under `go test -race`. This is empirical support for sharing
being safe at the Go-binding level; it is **not** a formal guarantee, since
Go's race detector cannot see races inside the underlying C/Rust runtime.
Treat as "no red flag found," not "proven safe" — validate further (e.g.
longer soak run, upstream wasmtime issue search) before fully trusting it in
production.

### 2.6 Scope of sharing: goroutines vs. processes vs. hosts

§2.5's shared `*wasmtime.Module` only crosses **goroutines inside one OS
process**. It is a live Go object wrapping native JIT-compiled code mapped
into that process's address space — it cannot be handed to another tier2
process, container, or host; there is no IPC/shared-memory path for it in
wasmtime-go.

What *can* cross a process/host boundary is the **serialized byte form**:
`Module.Serialize()` → bytes → written to storage any other tier2
process/host can reach → `NewModuleDeserialize()`/`NewModuleDeserializeFile()`
on the receiving side, compiling nothing, provided that side runs the same
wasmtime-go version and engine config (unenforced by the library — the cache
key, §4, must pin both or a mismatch fails/misbehaves silently per the
undocumented-further note in §2.5). This is exactly options **B** (node-local
disk, shared only by processes/pods that land on the same node) and **C**
(fleet-shared: the same serialize/deserialize bytes, but written to and read
from a store every host can reach — object storage or a small cache
service). Option **A** (in-process `Module` cache) never leaves one process;
it is the cheap floor, not the cross-host answer.

So: no, a compiled `*wasmtime.Module` by itself does not help two tier2 jobs
in different containers/hosts. Getting that requires B or C's
serialize-to-shared-storage path, not goroutine-level sharing.

## 3. Options

| Option | Scope of sharing | What it needs | Risk |
|---|---|---|---|
| **A. In-process LRU of compiled `*wasmtime.Module`, one shared `Engine` per tier2 process** | one process only | bounded-size cache keyed by content hash (not `moduleKey`/binary index, which isn't stable across requests); evict by total bytes, sized off §2.4 | low — contained to one process, no new infra; only helps when the *same* process handles repeat jobs for the *same* binary |
| **B. Node-local on-disk cache** (`Module.Serialize`/`NewModuleDeserializeFile`, keyed by `sha256(wasmCode) + wasmtime version + engine config hash`) | processes/pods on one node (if scheduler gives any locality) | local scratch dir, integrity check (only ever deserialize files this process wrote, under its own hash-named path — never deserialize arbitrary/foreign files) | medium — must pin wasmtime-go version + engine config in the key or deserialize will reject/misbehave on mismatch; still no benefit across nodes |
| **C. Fleet-shared cache** (same serialize/deserialize mechanism, backed by a small network store — object storage or a cache service) | entire tier2 fleet | population/eviction policy, a place to write to, and a latency budget check (fetch-over-network must beat recompile) | highest infra cost; biggest win if tier2 pods are scheduled with no package locality (this needs to be confirmed against the real tier2 scheduler — see Open Questions) |
| **D. wasmtime's built-in disk cache** (`Config.CacheConfigLoadDefault`/`CacheConfigLoad`) | same scope as B unless pointed at shared storage | one line of config, point at a dir | wasmtime's own cache is tuned for its own use cases (e.g. CLI); less control over key format/eviction than rolling our own `Serialize`/`Deserialize`; worth a quick trial but B is probably more predictable |
| **E. Revive `pipeline/cache.ModuleCache`** | — | — | **not recommended as-is**: it caches `wasm.Module` by a hash computed once per pipeline (not shared across pipelines/requests), has a background-eviction goroutine started per construction (never stopped — no `Close`/cancel observed), and nothing reads from it. Delete it; don't resurrect dead code when a proper shared cache (A/B/C) replaces its purpose outright. |

**You flagged, correctly, that option A alone is of limited value**: tier2 is a
fleet of many pods, and a per-process cache only pays off when the same
process happens to get repeat jobs for the same binary (same process = same
pod across the lifetime of a backfill/stream, not across the whole fleet).
Whether that's "limited" or "most of the win" depends on tier2's actual
scheduling/assignment model — see Open Questions below; this wasn't nailed
down in this investigation. Recommendation, in order, assuming no change to
scheduling:

1. **A first, unconditionally** — it's nearly free (no infra, bounded memory
   per §2.4, win is proportional only to request-affinity within a process)
   and removes the "no cache at all" baseline risk regardless of what else
   gets built.
2. **B next** — on-disk serialize/deserialize, keyed by content hash + wasmtime
   version + engine config. Gets node-local sharing "for free" if the node
   already has the artifact from a previous job, with no network dependency.
3. **C only if the fleet's job-to-pod assignment is proven to scatter requests
   for the same package across many different nodes/pods with no locality** —
   that's the condition that makes A+B insufficient and justifies the extra
   infrastructure and operational surface of a shared store.

## 4. Eviction / sizing

- Content-hash key (`sha256` of the wasm bytes), not `(binaryIndex, binaryType)`
  — the latter is only stable within one pipeline's own numbering.
- Bound the in-memory LRU by total bytes retained, not entry count, using the
  ~4.5–5x-source-size ratio from §2.4 to translate "how many distinct packages
  do we expect resident at once" into a memory budget.
- `SUBSTREAMS_WASM_CACHE_ENABLED` (`registry.go:80`) is an unrelated,
  explicitly non-production **instance** cache (reuses a `wasm.ModuleInstance`'s
  memory/state across calls — correctness-sensitive, deliberately disabled in
  production) and should not be conflated with a compilation cache; the new
  work doesn't touch it.

## 5. Interaction with the empty-segment-skip change

Another session is moving tier2's empty-segment skip earlier, ahead of store
loading and module compilation. That removes compile cost entirely for
segments that get skipped. A compilation cache (any option above) still pays
off for segments that *do* run, which is the majority case and where the
absolute per-job saving (§2.3) is smaller per job but recurs across every
tier2 job in the fleet. No code overlap: the skip logic is in
`service/tier2.go:601-631`; a compile cache sits inside `getWASMRegistry`
(`tier2.go:373`) and `wasm/wasmtime/module.go`, upstream of where the skip
check runs today and unaffected by where it moves to.

## 6. Open questions (not resolved here)

- tier2's actual pod/job scheduling model: does the same binary's segments
  tend to land on the same pod (locality helps A/B), or are they scattered
  fleet-wide (needed to justify C)? (Raised as "likely" from experience, not
  yet confirmed against the real scheduler — see §0.)
- Real empty-vs-non-empty segment ratio and real job-duration distribution
  from production tier2 metrics (not available in this sandboxed
  investigation) — needed to turn §2.3's per-job estimate into a fleet-wide
  cost/benefit number.
- Whether wasmtime's engine config ever varies per-request in tier2 (it
  currently doesn't — `wasm/wasmtime/module.go:26` always uses a bare
  `wasmtime.NewConfig()` — so a single shared `Engine`'s config is safe to fix
  once per process).

## 7. Phased plan

0. **Measure before building anything.** Export `loadingModuleDuration`
   (`pipeline.go:972`) as a real metric from tier2 (sum/histogram, tagged by
   package or binary hash); let it run for a few days; compare the aggregate
   against tier2's total fleet CPU. If it's noise, stop — §0 explains why. If
   it's a meaningful slice, this also answers §6's scheduling-locality
   question for free (repeat-hash hits on the same pod are directly visible
   once the metric exists), which decides whether step 2 alone is enough or
   step 3 is needed.
1. Delete dead `pipeline/cache.ModuleCache` and the inert `p.moduleCache`
   field/write-site — this has zero risk and zero dependency on step 0's
   answer, so it can happen regardless.
2. Add option A: one `wasmtime.Engine` + content-hash-keyed, byte-bounded LRU
   of compiled `*wasmtime.Module`, scoped to the tier2 process lifetime,
   replacing today's "new Engine + new Module every `getWASMRegistry` call."
   Cheapest option, and the one step 0's rescheduling-locality hypothesis
   argues for first.
3. Add option B on top, **only if step 0's data justifies it**: serialize new
   compiles to node-local disk (`Module.Serialize`), keyed by
   `sha256(wasmCode)-<wasmtime version>`; deserialize-on-miss from that path
   only (never from an arbitrary/untrusted path); fall back to normal compile
   on any deserialize error (version mismatch, corruption).
4. Re-measure in production before deciding on C — same aggregate-cost
   question as step 0, now with A (and maybe B) already removing some of the
   cost.
