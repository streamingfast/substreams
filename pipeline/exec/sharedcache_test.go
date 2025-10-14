// pipeline/exec/sharedcache_test.go
package exec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
)

type stubInstance struct{}

func (stubInstance) Cleanup(ctx context.Context) error { return nil }
func (stubInstance) Close(ctx context.Context) error   { return nil }

type stubModule struct{}

func (stubModule) NewInstance(ctx context.Context) (wasm.Instance, error) {
	return stubInstance{}, nil
}

func (stubModule) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument, argValues map[string][]byte) (wasm.Instance, error) {
	// No-op execution; return a non-nil instance so Execute() can call Close()
	return stubInstance{}, nil
}

func (stubModule) Close(ctx context.Context) error { return nil }

// slowStubModule simulates slow WASM execution to test race conditions
type slowStubModule struct {
	delay   int           // milliseconds to sleep
	started chan struct{} // signals when execution has started
}

func (s *slowStubModule) NewInstance(ctx context.Context) (wasm.Instance, error) {
	return stubInstance{}, nil
}

func (s *slowStubModule) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument, argValues map[string][]byte) (wasm.Instance, error) {
	if s.started != nil {
		close(s.started)
		s.started = nil // only signal once
	}
	// Simulate slow execution with actual delay
	time.Sleep(time.Duration(s.delay) * time.Millisecond)
	return stubInstance{}, nil
}

func (s *slowStubModule) Close(ctx context.Context) error { return nil }

// ---- helpers ----

func seedReqCtx(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{UniqueID: 1})
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, zap.NewNop()))
	return ctx
}

// ---- tests ----

// Repro of the original bug: same hash, same block, different entrypoint -> panic
// This requires concurrent execution to hit the race condition
func TestSharedCache_EntryPointCollision(t *testing.T) {
	cache := NewSharedCache(15)
	ctx := seedReqCtx(t)

	clock := &pbsubstreams.Clock{Id: "block-id", Number: 23559862}
	modHash := "same-wasm-binary-hash"

	// Use a slow module that blocks execution for a long time
	started := make(chan struct{})
	slowMod := &slowStubModule{delay: 500, started: started} // 500ms delay

	// Start first call in background - it will block for 500ms
	done := make(chan error, 1)
	go func() {
		callA := wasm.NewCall(clock, "pkg", "map_adr_messages", nil, nil, true, nil)
		done <- cache.Execute(ctx, slowMod, modHash, callA, nil, nil, nil)
	}()

	// Wait for first call to start executing
	<-started

	// Give it a tiny bit more time to ensure it's locked
	time.Sleep(50 * time.Millisecond)

	// Inspect what's in the cache after first call started
	cache.Lock()
	t.Logf("Cache entries for clock: %+v", cache.callEntries[cacheClock{id: clock.Id, num: clock.Number}])
	entry, exists := cache.callEntries[cacheClock{id: clock.Id, num: clock.Number}][modHash]
	cache.Unlock()

	if exists {
		entry.RLock()
		t.Logf("Found cache entry: moduleName=%s, entrypoint=%s", entry.moduleName, entry.entrypoint)
		entry.RUnlock()
	}

	// Now second call with different entrypoint should find the cache entry,
	// wait for first to finish, then panic when applying the cached result
	callB := wasm.NewCall(clock, "pkg", "map_flattened_messages", nil, nil, true, nil)
	t.Logf("Second call: moduleName=%s, entrypoint=%s", callB.ModuleName, callB.Entrypoint)

	// Create a new slowStubModule for the second call (doesn't need to signal)
	slowMod2 := &slowStubModule{delay: 0, started: nil}

	assert.Panics(t, func() {
		_ = cache.Execute(ctx, slowMod2, modHash, callB, nil, nil, nil)
	}, "expected applyResult to panic on entrypoint mismatch")

	// Wait for first call to complete
	err := <-done
	assert.NoError(t, err)
}

// This test demonstrates what SHOULD happen after the fix:
// Same hash + different entrypoint should NOT panic (just log a warning)
func TestSharedCache_EntryPointCollision_ShouldNotPanic_AfterFix(t *testing.T) {
	t.Skip("This test will pass after the fix is applied - it shows the desired behavior")

	cache := NewSharedCache(15)
	ctx := seedReqCtx(t)

	clock := &pbsubstreams.Clock{Id: "block-id", Number: 23559862}
	modHash := "same-wasm-binary-hash"

	started := make(chan struct{})
	slowMod := &slowStubModule{delay: 500, started: started}

	done := make(chan error, 1)
	go func() {
		callA := wasm.NewCall(clock, "pkg", "map_adr_messages", nil, nil, true, nil)
		done <- cache.Execute(ctx, slowMod, modHash, callA, nil, nil, nil)
	}()

	<-started
	time.Sleep(50 * time.Millisecond)

	callB := wasm.NewCall(clock, "pkg", "map_flattened_messages", nil, nil, true, nil)
	slowMod2 := &slowStubModule{delay: 0, started: nil}

	// After fix: should NOT panic, just log warning and use cached result
	assert.NotPanics(t, func() {
		_ = cache.Execute(ctx, slowMod2, modHash, callB, nil, nil, nil)
	}, "after fix, entrypoint mismatch should only log a warning, not panic")

	err := <-done
	assert.NoError(t, err)
}

// Simulate real production scenario: multiple modules from same package
// This mimics the evm-adr-messages case where map_adr_messages and other modules
// might be processed concurrently on the same block
func TestSharedCache_MultipleModulesConcurrent_ProductionScenario(t *testing.T) {
	cache := NewSharedCache(15)
	ctx := seedReqCtx(t)

	clock := &pbsubstreams.Clock{Id: "274b48f0e30910b07742fa8f218dd37a1a1bdeea1b8a6aa4e91ddbba9940deb0", Number: 23559862}

	// Simulate two different modules with different hashes (as they should be)
	// In production, these would have different entrypoints and different hashes
	hashMapAdr := "fbef15cbda4a4843a0969530e802c20c1e0b5dca"   // map_adr_messages
	hashFiltered := "b346114d6af1d3a10959f8ca96cfe4a43cf56cc5" // filtered_flattened_messages_no_index

	started1 := make(chan struct{})
	started2 := make(chan struct{})
	slowMod1 := &slowStubModule{delay: 300, started: started1}
	slowMod2 := &slowStubModule{delay: 100, started: started2}

	// Start first module execution
	done1 := make(chan error, 1)
	go func() {
		call1 := wasm.NewCall(clock, "evm_adr_messages", "map_adr_messages", nil, nil, true, nil)
		done1 <- cache.Execute(ctx, slowMod1, hashMapAdr, call1, nil, nil, nil)
	}()

	<-started1
	time.Sleep(50 * time.Millisecond)

	// Inspect cache - with bug, entry has empty moduleName/entrypoint
	cache.Lock()
	entry, exists := cache.callEntries[cacheClock{id: clock.Id, num: clock.Number}][hashMapAdr]
	cache.Unlock()

	if exists {
		entry.RLock()
		t.Logf("map_adr_messages cache entry: moduleName='%s', entrypoint='%s'", entry.moduleName, entry.entrypoint)
		entry.RUnlock()
		// With the bug (before fix): these would be empty ""
		// After fix: these should be "evm_adr_messages" and "map_adr_messages"
	}

	// Start second module execution concurrently
	done2 := make(chan error, 1)
	go func() {
		call2 := wasm.NewCall(clock, "evm_adr_messages", "filtered_flattened_messages_no_index", nil, nil, true, nil)
		done2 <- cache.Execute(ctx, slowMod2, hashFiltered, call2, nil, nil, nil)
	}()

	// Both should complete without panic since they have different hashes
	err1 := <-done1
	err2 := <-done2
	assert.NoError(t, err1)
	assert.NoError(t, err2)

	// Verify both entries are in cache with correct values
	cache.Lock()
	entry1 := cache.callEntries[cacheClock{id: clock.Id, num: clock.Number}][hashMapAdr]
	entry2 := cache.callEntries[cacheClock{id: clock.Id, num: clock.Number}][hashFiltered]
	cache.Unlock()

	assert.NotNil(t, entry1)
	assert.NotNil(t, entry2)
	assert.Equal(t, "evm_adr_messages", entry1.moduleName)
	assert.Equal(t, "map_adr_messages", entry1.entrypoint)
	assert.Equal(t, "evm_adr_messages", entry2.moduleName)
	assert.Equal(t, "filtered_flattened_messages_no_index", entry2.entrypoint)
}

// This test demonstrates the bug: if moduleHash incorrectly collides,
// the uninitialized entrypoint causes a panic
func TestSharedCache_ModuleHashCollision_CausesPanic(t *testing.T) {
	cache := NewSharedCache(15)
	ctx := seedReqCtx(t)

	clock := &pbsubstreams.Clock{Id: "block-id", Number: 12345}

	// Simulate a hash collision (or bug in hash computation)
	// where two different modules get the same hash
	collidingHash := "same-hash-different-modules"

	started := make(chan struct{})
	slowMod := &slowStubModule{delay: 300, started: started}

	// First module starts executing
	done1 := make(chan error, 1)
	go func() {
		call1 := wasm.NewCall(clock, "pkg", "module_A", nil, nil, true, nil)
		done1 <- cache.Execute(ctx, slowMod, collidingHash, call1, nil, nil, nil)
	}()

	<-started
	time.Sleep(50 * time.Millisecond)

	cache.Lock()
	entry := cache.callEntries[cacheClock{id: clock.Id, num: clock.Number}][collidingHash]
	cache.Unlock()

	entry.RLock()
	entrypointBeforeCompletion := entry.entrypoint
	entry.RUnlock()

	t.Logf("Entrypoint before first execution completes: '%s' (should be empty without fix, 'module_A' with fix)", entrypointBeforeCompletion)

	// Second module with different entrypoint but same hash tries to use cache
	call2 := wasm.NewCall(clock, "pkg", "module_B", nil, nil, true, nil)
	fastMod := &slowStubModule{delay: 0, started: nil}

	assert.Panics(t, func() {
		_ = cache.Execute(ctx, fastMod, collidingHash, call2, nil, nil, nil)
	}, "hash collision with different entrypoints should panic (demonstrates the bug)")

	err1 := <-done1
	assert.NoError(t, err1)
}

func TestSharedCache_KeyingAndPanicBehavior(t *testing.T) {
	cache := NewSharedCache(15)
	ctx := seedReqCtx(t)
	mod := stubModule{}

	clock := &pbsubstreams.Clock{Id: "block-id", Number: 23559862}

	hashA := "wasm-hash-A"
	hashB := "wasm-hash-B"

	// same hash + same entrypoint -> NO PANIC
	callA1 := wasm.NewCall(clock, "pkg", "entry_A", nil, nil, true, nil)
	assert.NoError(t, cache.Execute(ctx, mod, hashA, callA1, nil, nil, nil))
	callA2 := wasm.NewCall(clock, "pkg", "entry_A", nil, nil, true, nil)
	assert.NotPanics(t, func() {
		_ = cache.Execute(ctx, mod, hashA, callA2, nil, nil, nil)
	}, "same wasm hash and same entrypoint should not panic (cache hit)")

	// same hash + different entrypoint -> PANIC
	callB1 := wasm.NewCall(clock, "pkg", "entry_B", nil, nil, true, nil)
	assert.Panics(t, func() {
		_ = cache.Execute(ctx, mod, hashA, callB1, nil, nil, nil)
	}, "same wasm hash but different entrypoint should panic")

	// different hashes + different entrypoints -> NO PANIC
	callB2 := wasm.NewCall(clock, "pkg", "entry_B", nil, nil, true, nil)
	assert.NotPanics(t, func() {
		_ = cache.Execute(ctx, mod, hashB, callB2, nil, nil, nil)
	}, "different wasm hash means separate cache bucket; should not panic")
}
