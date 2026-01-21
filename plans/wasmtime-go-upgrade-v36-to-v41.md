# Implementation Plan: wasmtime-go Upgrade v36.0.0 to v41.0.0

## ULTIMATE GOAL

Upgrade the wasmtime-go dependency from v36.0.0 to v41.0.0 in the Substreams project while maintaining full backward compatibility with existing WASM modules and ensuring all tests pass. The upgrade should be seamless for end users with no breaking changes to Substreams' public API.

## Status: Ready for Implementation

## Executive Summary

This plan covers upgrading `github.com/bytecodealliance/wasmtime-go/v36` to `github.com/bytecodealliance/wasmtime-go/v41` across the Substreams codebase. Based on API analysis, **the wasmtime-go API appears to be stable between v36 and v41**, with no breaking changes to the core types used in this project (Engine, Store, Module, Linker, Instance, Memory, Func, Caller, Trap, Val).

The main risks are:
1. Potential behavioral changes in the underlying wasmtime runtime
2. WebAssembly Threads (SharedMemory) is now disabled by default in v40+ (not used by Substreams)
3. Build system compatibility (Rust 1.90.0+ now required for building wasmtime from source)

---

## Files Requiring Modification

### Primary Files (Import Path Updates Required)

| File | Description | Change Type |
|------|-------------|-------------|
| `go.mod` (line 40) | Main dependency declaration | Version bump |
| `wasm/wasmtime/module.go` | Core module compilation and engine setup | Import path update |
| `wasm/wasmtime/instance.go` | Instance creation, linking, exports | Import path update |
| `wasm/wasmtime/heap.go` | Memory management | Import path update |
| `wasm/wasmtime/state_externs.go` | State import functions | No changes (uses local types only) |
| `wasm/wasmtime/logging.go` | Logging shims | No changes (no wasmtime imports) |

### Secondary Files (E2E Tests)

| File | Description | Change Type |
|------|-------------|-------------|
| `tests_e2e/go.mod` (line 54) | E2E test dependency (indirect) | Version bump via main module |

---

## API Compatibility Analysis

### APIs Used in Substreams

| API | v36 Signature | v41 Signature | Status |
|-----|---------------|---------------|--------|
| `wasmtime.NewConfig()` | `func NewConfig() *Config` | Unchanged | Compatible |
| `wasmtime.NewEngineWithConfig(cfg)` | `func NewEngineWithConfig(config *Config) *Engine` | Unchanged | Compatible |
| `wasmtime.NewStore(engine)` | `func NewStore(engine *Engine) *Store` | Unchanged | Compatible |
| `store.GC()` | `func (store *Store) GC()` | Unchanged | Compatible |
| `store.Close()` | `func (store *Store) Close()` | Unchanged | Compatible |
| `wasmtime.NewModule(engine, wasmCode)` | `func NewModule(engine *Engine, wasm []byte) (*Module, error)` | Unchanged | Compatible |
| `wasmtime.NewLinker(engine)` | `func NewLinker(engine *Engine) *Linker` | Unchanged | Compatible |
| `linker.FuncWrap(module, name, f)` | `func (l *Linker) FuncWrap(module, name string, f interface{}) error` | Unchanged | Compatible |
| `linker.Define(store, module, name, item)` | `func (l *Linker) Define(store Storelike, module, name string, item AsExtern) error` | Unchanged | Compatible |
| `linker.Instantiate(store, module)` | `func (l *Linker) Instantiate(store Storelike, module *Module) (*Instance, error)` | Unchanged | Compatible |
| `linker.Get(store, module, name)` | `func (l *Linker) Get(store Storelike, module, name string) *Extern` | Unchanged | Compatible |
| `linker.Close()` | `func (l *Linker) Close()` | Unchanged | Compatible |
| `instance.GetExport(store, name)` | `func (i *Instance) GetExport(store Storelike, name string) *Extern` | Unchanged | Compatible |
| `wasmtime.NewFunc(store, funcType, callback)` | `func NewFunc(store Storelike, ty *FuncType, f func(*Caller, []Val) ([]Val, *Trap)) *Func` | Unchanged | Compatible |
| `func.Call(store, args...)` | `func (f *Func) Call(store Storelike, args ...interface{}) (interface{}, error)` | Unchanged | Compatible |
| `memory.UnsafeData(store)` | `func (mem *Memory) UnsafeData(store Storelike) []byte` | Unchanged | Compatible |
| `engine.Close()` | `func (engine *Engine) Close()` | Unchanged | Compatible |
| `wasmtime.Caller` | Type used in callbacks | Unchanged | Compatible |
| `wasmtime.Val` | Value type | Unchanged | Compatible |
| `wasmtime.Trap` | Trap/error type | Unchanged | Compatible |
| `module.Imports()` | `func (m *Module) Imports() []*ImportType` | Unchanged | Compatible |
| `importType.Module()` | Method on ImportType | Unchanged | Compatible |
| `importType.Name()` | Method on ImportType | Unchanged | Compatible |
| `importType.Type().FuncType()` | Method chain | Unchanged | Compatible |

### Disabled Features (Not Used)

| Feature | Status in v40+ | Substreams Usage |
|---------|----------------|------------------|
| WebAssembly Threads | Disabled by default | Not used |
| SharedMemory | Disabled by default | Not used |
| Fuel consumption | Commented out | Not affected |

---

## Implementation Tasks

### Priority 1: Pre-Upgrade Verification

- [ ] **P1.1** Run all existing tests on current v36 to establish baseline
  - Command: `go test ./wasm/...`
  - Command: `go test ./...` (full test suite)
  - Rationale: Ensure we have a known-good state before any changes

- [ ] **P1.2** Run E2E tests on current v36
  - Command: `cd tests_e2e && go test ./...`
  - Rationale: Verify E2E tests pass before upgrade

- [ ] **P1.3** Verify build succeeds with current version
  - Command: `go build ./...`
  - Rationale: Confirm current build state is healthy

### Priority 2: Dependency Update

- [ ] **P2.1** Update `go.mod` to use wasmtime-go v41
  - File: `go.mod`
  - Change line 40 from:
    ```
    github.com/bytecodealliance/wasmtime-go/v36 v36.0.0
    ```
    to:
    ```
    github.com/bytecodealliance/wasmtime-go/v41 v41.0.0
    ```

- [ ] **P2.2** Update import path in `wasm/wasmtime/module.go`
  - File: `wasm/wasmtime/module.go`
  - Change line 7 from:
    ```go
    wasmtime "github.com/bytecodealliance/wasmtime-go/v36"
    ```
    to:
    ```go
    wasmtime "github.com/bytecodealliance/wasmtime-go/v41"
    ```

- [ ] **P2.3** Update import path in `wasm/wasmtime/instance.go`
  - File: `wasm/wasmtime/instance.go`
  - Change line 7 from:
    ```go
    wasmtime "github.com/bytecodealliance/wasmtime-go/v36"
    ```
    to:
    ```go
    wasmtime "github.com/bytecodealliance/wasmtime-go/v41"
    ```

- [ ] **P2.4** Update import path in `wasm/wasmtime/heap.go`
  - File: `wasm/wasmtime/heap.go`
  - Change line 7 from:
    ```go
    "github.com/bytecodealliance/wasmtime-go/v36"
    ```
    to:
    ```go
    "github.com/bytecodealliance/wasmtime-go/v41"
    ```

- [ ] **P2.5** Run `go mod tidy` to update dependencies
  - Command: `go mod tidy`
  - Rationale: Ensure go.sum is updated and transitive dependencies are resolved

### Priority 3: Post-Upgrade Verification

- [ ] **P3.1** Verify code compiles with new version
  - Command: `go build ./...`
  - Rationale: Catch any API incompatibilities at compile time

- [ ] **P3.2** Run unit tests for wasm package
  - Command: `go test ./wasm/...`
  - Rationale: Verify wasm-specific functionality works

- [ ] **P3.3** Run full test suite
  - Command: `go test ./...`
  - Rationale: Ensure no regressions across the entire codebase

- [ ] **P3.4** Run E2E tests
  - Command: `cd tests_e2e && go test ./...`
  - Rationale: Verify end-to-end functionality with real WASM modules

- [ ] **P3.5** Run benchmarks to compare performance
  - Command: `go test -bench=. ./wasm/bench/...`
  - Rationale: Ensure no performance regression from the upgrade

### Priority 4: Documentation and Finalization

- [ ] **P4.1** Update CHANGELOG/release notes
  - File: `docs/release-notes/change-log.md`
  - Add entry noting wasmtime-go upgrade from v36.0.0 to v41.0.0

- [ ] **P4.2** Test with real Substreams modules
  - Run several production Substreams modules through the updated runtime
  - Verify output matches expected results
  - Rationale: Ensure behavioral compatibility with actual user modules

---

## Risk Assessment

### Low Risk Items

| Item | Rationale |
|------|-----------|
| Import path changes | Mechanical change, no API differences |
| Basic types (Engine, Store, Module) | API unchanged between versions |
| Memory access (UnsafeData) | API unchanged, same safety requirements |
| Linker operations | API unchanged |

### Medium Risk Items

| Item | Rationale | Mitigation |
|------|-----------|------------|
| Performance differences | New version may have different optimization levels | Run benchmarks before/after |
| Trap handling behavior | May have subtle changes | Verify error messages in tests |
| WASM compilation edge cases | Cranelift optimizations may differ | E2E testing with real modules |

### Low-to-None Risk Items (Not Used)

| Item | Rationale |
|------|-----------|
| WebAssembly Threads | Not used in Substreams |
| SharedMemory | Not used in Substreams |
| Component Model | Not used in Substreams |
| Async operations | Not used in Substreams |
| WASI | Not used in current implementation |
| Fuel consumption | Currently disabled (commented out) |

---

## Rollback Plan

If issues are discovered after the upgrade:

1. **Immediate Rollback**
   - Revert `go.mod` to use `wasmtime-go/v36 v36.0.0`
   - Revert all import path changes in:
     - `wasm/wasmtime/module.go`
     - `wasm/wasmtime/instance.go`
     - `wasm/wasmtime/heap.go`
   - Run `go mod tidy`
   - Verify tests pass

2. **Issue Investigation**
   - Document the specific failure
   - Check wasmtime-go release notes for related changes
   - Consider intermediate versions (v37, v38, v39, v40) if v41 has specific issues

---

## Version Changelog Summary (v37-v41)

### v37.0.0 (September 2025)
- GC configuration in Config
- Pooling allocator metrics
- Minimum Rust 1.87.0 required

### v38.0.0 (October 2025)
- Minor release with bug fixes

### v39.0.0 (November 2025)
- WebAssembly exceptions in C API
- Callback-based stdout/stderr for WASI
- Minimum Rust 1.89.0 required

### v40.0.0 (December 2025)
- **WebAssembly Threads disabled by default**
- **SharedMemory disabled by default**
- WASIp3 HTTP support
- OOM handling groundwork

### v41.0.0 (January 2026)
- Breakpoint and single-stepping via debug feature
- Enhanced Future/Stream support
- Minimum Rust 1.90.0 required

---

## Estimated Effort

| Phase | Estimated Time |
|-------|----------------|
| Pre-upgrade verification | 15-30 minutes |
| Dependency updates | 10 minutes |
| Post-upgrade verification | 30-60 minutes |
| Documentation | 10 minutes |
| **Total** | **1-2 hours** |

---

## Completed Items

(Items will be moved here as they are completed)

---

## Notes

- The wasmtime-go bindings follow the main wasmtime Rust release cycle
- Go bindings are generated from the C API, so breaking changes are rare
- The project does not use any of the features that had breaking changes (threads, async, component model)
- All tests should be run with `-race` flag to catch any potential data race issues introduced by the upgrade
