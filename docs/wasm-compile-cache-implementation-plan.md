# WASM Compile Cache (tier2, option A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give tier2 an in-process, memory-bounded cache of compiled `*wasmtime.Module`s, keyed by content hash, sharing one long-lived `wasmtime.Engine`, so a tier2 process that handles repeat jobs for the same binary (same pod reused across a backfill/stream) stops recompiling it from scratch every time. Wasmtime only — wazero is not used in production.

**Architecture:** `wasm.Registry` gains an opaque, optional `moduleCache any` slot set via a new `RegistryOption`; `wasm/wasmtime`'s module factory checks that slot and, when present, routes compilation through a `wasm/wasmtime.ModuleCache` (one shared `Engine`, `sha256`-keyed, byte-weight-bounded LRU, `singleflight`-deduped) instead of creating a fresh `Engine` + `Module` per call. `service.Tier2Service` is the only call site that ever constructs and passes that cache — every other caller (tier1, CLI, sinks) is structurally unaffected. The option defaults unset, so shipped behavior is unchanged until something opts in.

**Tech Stack:** Go 1.26, `github.com/bytecodealliance/wasmtime-go/v41`, `golang.org/x/sync/singleflight` (already a dependency), testify `require`/`assert`.

**Spec:** `docs/wasm-compile-cache-investigation.md` (measurements §2, options §3, this plan implements option A only; §4 eviction/sizing; §0 explains why this ships default-off pending production measurement).

## Global Constraints

- Go 1.26 idioms throughout: `for range n`, `t.Context()`, `wg.Go`, etc. (per `modern-go-guidelines:use-modern-go`).
- `github.com/stretchr/testify/require` / `assert` for all assertions; never `t.Fatal`.
- No new third-party dependency — `golang.org/x/sync v0.23.0` (singleflight) is already in `go.mod`.
- Zero behavior change for any caller that does not opt in: tier1, `substreams run` CLI, sinks must compile and run identically to today, since they never pass the new option.
- Cache key is `sha256(wasmCode)` only (§4 of the spec) — never `(binaryIndex, binaryType)`, which is only stable within one pipeline's own numbering.
- One `wasmtime.Engine` per `ModuleCache`, created once, never per cached entry (spec §2.4: a fresh engine per module is ~7x the memory of a shared one).
- Ship disabled by default (no call site passes `WithWASMModuleCacheMaxBytes` yet) — turning it on in production is a follow-up decision gated by the spec's §0/§7 step 0 measurement, not part of this plan.
- Does not touch the empty-segment-skip reordering in `service/tier2.go:601-631` — separate, in-flight work.

## Review Focus

- Two tier2 goroutines request the same not-yet-cached binary concurrently — must compile exactly once, not twice (wasted CPU, or worse, two distinct `*wasmtime.Module`s for the same key racing into the cache map).
- A single binary's estimated weight alone exceeds `maxBytes` — must still get cached (and usable), not dropped into a permanent always-recompile state or caught in an eviction loop that empties the cache on every insert.
- `service/tier2.go:1113`'s existing `mod.Close(ctx)` (called at pipeline teardown) must not close the shared `Engine` when the module came from the cache — a later cache hit against a closed engine must not panic or error.
- With no option passed (`s.wasmModuleCache == nil`, the default), `getWASMRegistry` must produce byte-for-byte the same registry construction as before this change — no accidental always-on behavior.
- Two different `wasmCodeType` strings (e.g. `wasm/rust-v1` vs `wasm/rust-v1+wasm-bindgen-shims`) sharing byte-identical `wasmCode` must still produce a correctly instantiable module from the one cached compiled artifact — wasm-bindgen-shim wiring happens at instantiate time (`wasm/wasmtime/module.go`'s `newInstance`), not compile time, so this must not require two cache entries for one hash.

---

### Task 1: Opaque module-cache slot on `wasm.Registry`

**Files:**
- Modify: `wasm/registry.go:14-18` (struct), `wasm/registry.go:58` (`NewRegistry` signature)
- Test: `wasm/registry_test.go` (new file)

**Interfaces:**
- Produces: `type RegistryOption func(*Registry)`; `func WithModuleCache(cache any) RegistryOption`; `func (r *Registry) ModuleCache() any`; `func NewRegistry(extensions map[string]map[string]WASMExtension, opts ...RegistryOption) *Registry` (variadic addition, backward compatible with every existing call site).

The cache lives behind `any` here deliberately: `wasm` must not import `wasm/wasmtime` (which already imports `wasm` — a reverse import would cycle), so this package only stores and hands back an opaque reference. The concrete type assertion happens in Task 3, inside `wasm/wasmtime`.

- [ ] **Step 1: Write the failing test**

```go
// wasm/registry_test.go
package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry_ModuleCache(t *testing.T) {
	r := NewRegistry(nil)
	assert.Nil(t, r.ModuleCache())

	sentinel := "fake-cache"
	r2 := NewRegistry(nil, WithModuleCache(sentinel))
	assert.Equal(t, sentinel, r2.ModuleCache())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./wasm/ -run TestRegistry_ModuleCache -v`
Expected: FAIL — `WithModuleCache` undefined / `NewRegistry` does not accept a second argument.

- [ ] **Step 3: Implement in `wasm/registry.go`**

Add `moduleCache any` to the `Registry` struct (line 14-18). Add `RegistryOption`, `WithModuleCache`, and `ModuleCache()` as described in Interfaces above. Change `NewRegistry`'s signature to accept `opts ...RegistryOption` and apply them (loop calling each `opt(r)`) right after `r := &Registry{}` (`wasm/registry.go:72`), before the extensions loop — order relative to the extensions loop doesn't matter since they touch different fields, but applying early keeps the function's early lines in signature order.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./wasm/ -run TestRegistry_ModuleCache -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add wasm/registry.go wasm/registry_test.go
git commit -m "Add optional module-cache slot to wasm.Registry"
```

---

### Task 2: `wasm/wasmtime.ModuleCache` — shared-engine, weight-bounded LRU

**Files:**
- Create: `wasm/wasmtime/cache.go`
- Test: `wasm/wasmtime/cache_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 directly (this type is runtime-agnostic internally — it only wraps `*wasmtime.Module`/`*wasmtime.Engine`).
- Produces: `type ModuleCache struct{...}` with an exported `engine *wasmtime.Engine` field (package-internal, used by Task 3's `newModule`); `func NewModuleCache(maxBytes uint64) *ModuleCache`; `func (c *ModuleCache) getOrCompile(wasmCode []byte) (*wasmtime.Module, error)`.

Weight estimate: `weight := uint64(len(wasmCode)) * weightMultiplier` with `const weightMultiplier = 5` (spec §2.4: measured ~4.5–5x source size with a shared engine; round up). Eviction: standard LRU (`container/list` + `map[string]*list.Element`), evict from the back while `curBytes > maxBytes`, but never evict the cache's only remaining entry (a single binary larger than `maxBytes` alone should still be cached once, not permanently refused caching). Key: `sha256(wasmCode)`, hex-encoded. Concurrency: `golang.org/x/sync/singleflight.Group`, one `Do` call keyed by the hash wrapping "check cache, compile on miss, insert"; re-check the cache inside the `Do` closure in case another caller inserted while this one waited to acquire the singleflight slot (true for the *first* caller to a given key, not a retry-after-miss subtlety — `singleflight.Do` already guarantees only one caller runs the function body per in-flight key, so the inner re-check only matters for callers arriving after the first one's function has already returned and the "forgotten" singleflight entry briefly overlaps a cache write — write the test in Step 1 first and let it tell you if the re-check is needed).

- [ ] **Step 1: Write the failing tests**

```go
// wasm/wasmtime/cache_test.go
package wasmtime

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleCache_CachesByContentHash(t *testing.T) {
	code := readTestWasm(t) // small valid .wasm fixture, see helper below
	cache := NewModuleCache(10 * 1024 * 1024)

	m1, err := cache.getOrCompile(code)
	require.NoError(t, err)

	m2, err := cache.getOrCompile(code)
	require.NoError(t, err)

	assert.Same(t, m1, m2, "same wasm bytes must return the same cached *wasmtime.Module, not recompile")
}

func TestModuleCache_ConcurrentMissesCompileOnce(t *testing.T) {
	code := readTestWasm(t)
	cache := NewModuleCache(10 * 1024 * 1024)

	const n = 20
	results := make([]*wasmtime.Module, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			results[i], errs[i] = cache.getOrCompile(code)
		})
	}
	wg.Wait()

	for i := range n {
		require.NoError(t, errs[i])
		assert.Same(t, results[0], results[i])
	}
}

func TestModuleCache_EvictsLeastRecentlyUsed(t *testing.T) {
	small := readTestWasm(t)
	large := readLargerTestWasm(t) // a second, differently-sized fixture

	// budget big enough for exactly one of these at a time
	cache := NewModuleCache(uint64(len(large)) * weightMultiplier)

	_, err := cache.getOrCompile(small)
	require.NoError(t, err)
	_, err = cache.getOrCompile(large)
	require.NoError(t, err)

	assert.Equal(t, 1, cache.len(), "inserting large should have evicted small to stay under budget")
}

func TestModuleCache_OversizedSingleEntryStillCached(t *testing.T) {
	code := readTestWasm(t)
	cache := NewModuleCache(1) // smaller than any real compiled module's weight

	m1, err := cache.getOrCompile(code)
	require.NoError(t, err)
	m2, err := cache.getOrCompile(code)
	require.NoError(t, err)
	assert.Same(t, m1, m2, "a single entry over budget must still be cached, not recompiled every call")
}
```

Add a small unexported `(c *ModuleCache) len() int` accessor (returns `len(c.entries)`) purely for the eviction test above. Add `readTestWasm`/`readLargerTestWasm` test helpers reading from `../bench/substreams_wasm/substreams.wasm` and `../bench/testdata/...` or embed a tiny hand-built `.wasm` fixture if reaching into `wasm/bench` from `wasm/wasmtime`'s tests feels wrong — either is fine, pick whichever keeps the import path simplest; no production code depends on the choice.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./wasm/wasmtime/ -run TestModuleCache -v`
Expected: FAIL — `NewModuleCache` undefined.

- [ ] **Step 3: Implement `wasm/wasmtime/cache.go`**

Implement `ModuleCache`, `NewModuleCache`, `getOrCompile`, `insert`, `len`, and a `contentHash` helper (`sha256.Sum256` → hex), per the Interfaces/weight/eviction description above. `NewModuleCache` constructs the single shared engine: `wasmtime.NewEngineWithConfig(wasmtime.NewConfig())` (same bare config `wasm/wasmtime/module.go:26-31` uses today, so compiled modules behave identically to the uncached path).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./wasm/wasmtime/ -run TestModuleCache -v -race`
Expected: PASS (run with `-race` specifically for the concurrent-misses test).

- [ ] **Step 5: Commit**

```bash
git add wasm/wasmtime/cache.go wasm/wasmtime/cache_test.go
git commit -m "Add content-hash-keyed, weight-bounded wasmtime module cache"
```

---

### Task 3: Wire the cache into the wasmtime module factory

**Files:**
- Modify: `wasm/wasmtime/module.go:14-19` (struct), `:25-56` (`newModule`), `:58-61` (`Close`)
- Test: `wasm/wasmtime/module_test.go` (new file)

**Interfaces:**
- Consumes: `Registry.ModuleCache() any` (Task 1), `*ModuleCache`/`getOrCompile` (Task 2).
- Produces: `Module.ownsEngine bool` field (new); unchanged `Module` public behavior otherwise.

Add `ownsEngine bool` to the `Module` struct (`module.go:14-19`). In `newModule`, move the `wasm.ParseWASMCodeType` call (`module.go:33-36`) above the engine/compile logic (pure reordering, same error path), then branch: if `registry.ModuleCache()` returns non-nil, type-assert to `*ModuleCache` (return a `fmt.Errorf` wrapping `%T` on assertion failure — should never happen since only this package's `WithModuleCache` call sites in the repo ever set it, but don't panic on a bad assertion), call `cache.getOrCompile(wasmCode)`, and build the returned `*Module` with `engine: cache.engine` and `ownsEngine: false`. Otherwise, keep today's exact fresh-engine path (`module.go:26,31,42-45`) and set `ownsEngine: true`. In `Close`, only call `m.engine.Close()` when `m.ownsEngine` is true.

- [ ] **Step 1: Write the failing tests**

```go
// wasm/wasmtime/module_test.go
package wasmtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/wasm"
)

func TestNewModule_NoCacheOption_UnchangedBehavior(t *testing.T) {
	code := readTestWasm(t)
	registry := wasm.NewRegistry(nil)

	mod, err := newModule(t.Context(), code, "wasm/rust-v1", registry)
	require.NoError(t, err)
	require.NoError(t, mod.Close(t.Context()))
}

func TestNewModule_WithCache_SharesEngineAndSurvivesClose(t *testing.T) {
	code := readTestWasm(t)
	cache := NewModuleCache(10 * 1024 * 1024)
	registry := wasm.NewRegistry(nil, wasm.WithModuleCache(cache))

	mod1, err := newModule(t.Context(), code, "wasm/rust-v1", registry)
	require.NoError(t, err)
	require.NoError(t, mod1.Close(t.Context()), "closing a cache-backed module must not error")

	// cache must still be usable after the first module's Close()
	mod2, err := newModule(t.Context(), code, "wasm/rust-v1", registry)
	require.NoError(t, err)
	assert.Same(t, mod1.(*Module).module, mod2.(*Module).module, "second call must hit the cache, not recompile")
	require.NoError(t, mod2.Close(t.Context()))
}

func TestNewModule_WithCache_BadCacheTypeErrors(t *testing.T) {
	registry := wasm.NewRegistry(nil, wasm.WithModuleCache("not-a-module-cache"))
	_, err := newModule(t.Context(), readTestWasm(t), "wasm/rust-v1", registry)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./wasm/wasmtime/ -run TestNewModule -v`
Expected: FAIL on the cache-aware tests (no cache branch exists yet); the no-cache test should already pass unchanged (confirms today's behavior before touching it).

- [ ] **Step 3: Implement the changes in `wasm/wasmtime/module.go`**

As described in Interfaces above.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./wasm/wasmtime/ -v`
Expected: PASS, including Task 2's tests (same package).

- [ ] **Step 5: Commit**

```bash
git add wasm/wasmtime/module.go wasm/wasmtime/module_test.go
git commit -m "Route wasmtime module compilation through an optional shared cache"
```

---

### Task 4: Tier2-only activation

**Files:**
- Modify: `service/options.go` (new `Option`), `service/tier2.go:87-113` (struct field), `:373-382` (`getWASMRegistry`)
- Test: `service/tier2_wasm_module_cache_test.go` (new file)

**Interfaces:**
- Consumes: `wasmtime.NewModuleCache` (Task 2), `wasm.WithModuleCache`/`wasm.RegistryOption` (Task 1).
- Produces: `func WithWASMModuleCacheMaxBytes(maxBytes uint64) Option`; `Tier2Service.wasmModuleCache *wasmtime.ModuleCache` field (nil by default — the only state that matters for "tier2-only, opt-in, default off").

Add `wasmModuleCache *wasmtime.ModuleCache` to `Tier2Service` (`service/tier2.go`, alongside the other tier2-only fields around line 100-113). Add `WithWASMModuleCacheMaxBytes` to `service/options.go` following the exact shape of `WithSegmentExecutionTimeout` (same file): a `case *Tier1Service: // not used` branch and a `case *Tier2Service:` branch that does `s.wasmModuleCache = wasmtime.NewModuleCache(maxBytes)`. This needs `"github.com/streamingfast/substreams/wasm/wasmtime"` imported in `service/options.go` (no name collision with anything already imported there). Modify `getWASMRegistry` (`service/tier2.go:373-382`) to build `var opts []wasm.RegistryOption` and append `wasm.WithModuleCache(s.wasmModuleCache)` only when `s.wasmModuleCache != nil`, then call `wasm.NewRegistry(exts, opts...)`.

- [ ] **Step 1: Write the failing tests**

```go
// service/tier2_wasm_module_cache_test.go
package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTier2Service_getWASMRegistry_NoOptionByDefault(t *testing.T) {
	s := &Tier2Service{}
	registry, err := s.getWASMRegistry(nil)
	require.NoError(t, err)
	assert.Nil(t, registry.ModuleCache(), "without WithWASMModuleCacheMaxBytes, registry must carry no cache")
}

func TestTier2Service_getWASMRegistry_WithOption(t *testing.T) {
	s := &Tier2Service{}
	WithWASMModuleCacheMaxBytes(1024 * 1024)(s)

	registry, err := s.getWASMRegistry(nil)
	require.NoError(t, err)
	assert.NotNil(t, registry.ModuleCache())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./service/ -run TestTier2Service_getWASMRegistry -v`
Expected: FAIL — `WithWASMModuleCacheMaxBytes` undefined.

- [ ] **Step 3: Implement the changes in `service/options.go` and `service/tier2.go`**

As described in Interfaces above.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./service/ -run TestTier2Service_getWASMRegistry -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/options.go service/tier2.go service/tier2_wasm_module_cache_test.go
git commit -m "Let tier2 opt into the wasmtime module cache via WithWASMModuleCacheMaxBytes"
```

---

### Task 5: Prove it against the investigation's own numbers

**Files:**
- Modify: `wasm/bench/compile_bench_test.go` (add a cache-comparison benchmark)

**Interfaces:**
- Consumes: `wasm.NewRegistry`/`wasm.WithModuleCache` (Task 1), `wasmtime.NewModuleCache` (Task 2) — imports `github.com/streamingfast/substreams/wasm/wasmtime` directly (this file already blank-imports it for factory registration).

Add `BenchmarkCompile_Cached`, parallel in structure to the existing `BenchmarkCompile` (same file), that builds one `wasmtime.NewModuleCache(64<<20)`, builds one `wasm.NewRegistry(nil, wasm.WithModuleCache(cache))`, and times `registry.NewModule(ctx, bin.code, bin.codeType)` across `b.Loop()` for the same `bins` slice already defined in this file. This is the regression check: steady-state per-call cost on a cache hit should land near the existing `wasm/bench/shared_module_concurrency_test.go`'s implicit assumption (sub-millisecond, dominated by a map lookup + hash, not a recompile) — compare its reported `ns/op` against `BenchmarkCompile`'s wasmtime rows (§2.1 of the spec) to confirm the cache actually removes the compile cost it claims to.

- [ ] **Step 1: Write the benchmark**

```go
func BenchmarkCompile_Cached(b *testing.B) {
	bins := []compileBinary{
		{"bench_substreams.wasm", readCode(b, "substreams_wasm/substreams.wasm"), "wasm/rust-v1"},
	}
	bins = append(bins, spkgBinaries(b, "wasmbindgen_substreams", "../../test/testdata/wasmbindgen_substreams/wasmbindgen-substreams-v0.1.0.spkg")...)

	for _, bin := range bins {
		b.Run(fmt.Sprintf("bin=%s,bytes=%d", bin.tag, len(bin.code)), func(b *testing.B) {
			b.Setenv("SUBSTREAMS_WASM_RUNTIME", "wasmtime")
			ctx := context.Background()
			cache := wasmtime.NewModuleCache(64 << 20)
			registry := wasm.NewRegistry(nil, wasm.WithModuleCache(cache))
			_, err := registry.NewModule(ctx, bin.code, bin.codeType) // warm the cache; excluded from timing
			require.NoError(b, err)

			b.ReportAllocs()
			for b.Loop() {
				_, err := registry.NewModule(ctx, bin.code, bin.codeType)
				require.NoError(b, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./wasm/bench/ -run '^$' -bench BenchmarkCompile_Cached -benchtime 20x -v`
Expected: passes; `ns/op` for each binary is at least an order of magnitude below the matching `wasmtime` row in `BenchmarkCompile` for the same binary (spec §2.1: 8.7 ms / 11.5 ms uncached) — if it isn't, something in Task 3's wiring isn't actually hitting the cache and needs investigating before this task is considered done.

- [ ] **Step 3: Commit**

```bash
git add wasm/bench/compile_bench_test.go
git commit -m "Add cache-hit benchmark proving the wasmtime module cache removes recompiles"
```

## Self-Review

1. **Spec coverage:** §2.4's shared-engine requirement → Task 2 (`ModuleCache` owns one `Engine`). §2.5's goroutine-sharing claim → exercised by Task 2's concurrent test under `-race`. §2.6 (goroutines-only, not cross-host) → unchanged by this plan; no task claims cross-process sharing, matching option A's documented scope. §3's "evict by bytes using the 4.5–5x ratio" → Task 2's `weightMultiplier`. §4's "content-hash key, not `(binaryIndex, binaryType)`" → Task 2's `sha256`-only key, and Review Focus item 5 pins that two `wasmCodeType`s sharing bytes still work. §0/§7's "ship default-off, gated by production measurement" → Global Constraints and Task 4's nil-by-default field.
2. **Step scan:** no step leaves a signature or value undecided; the one place that could read as open-ended (singleflight re-check "let the test tell you") is deliberately a step-3 implementation choice bounded by a step-1 test that fails if it's wrong, not an unresolved requirement.
3. **Type consistency:** `wasm.RegistryOption`, `wasm.WithModuleCache(any)`, `Registry.ModuleCache() any` (Task 1) are the exact names Task 3 and Task 4 consume; `wasmtime.ModuleCache`, `wasmtime.NewModuleCache(uint64)`, `(*ModuleCache).getOrCompile([]byte) (*wasmtime.Module, error)` (Task 2) are the exact names Task 3 and Task 5 consume; `Module.ownsEngine` (Task 3) isn't referenced outside that task. Checked consistent across all five tasks.
4. **Review Focus:** all five items map to an explicit test: singleflight dedup → `TestModuleCache_ConcurrentMissesCompileOnce`; oversized single entry → `TestModuleCache_OversizedSingleEntryStillCached`; `Close()` not killing a shared engine → `TestNewModule_WithCache_SharesEngineAndSurvivesClose`; default-off → `TestTier2Service_getWASMRegistry_NoOptionByDefault`; shared hash across `wasmCodeType` variants → implied by Task 1/3's hash-only key and not given its own dedicated test — **gap**: add `TestModuleCache_CachesByContentHash` already covers same-bytes-same-key, but add one assertion in Task 3 Step 1 instantiating the *returned* module (call `NewInstance`/execute) rather than just closing it, so a cache hit is proven usable, not just reference-equal. Noted inline in Task 3; implementer should extend `TestNewModule_WithCache_SharesEngineAndSurvivesClose` with an actual `NewInstance` call if the plan reviewer agrees it's worth the fixture complexity — flagged here rather than silently assumed.
5. **Proportion:** five tasks, each with a handful of short Go snippets sized to what a fresh implementer needs (names, signatures, key values) and no full method bodies for anything the tests don't already pin — proportionate to a change this size (five files touched, no new third-party dependency, no cross-cutting rename).
