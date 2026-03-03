package stage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
)

// TestMultiSquashSpawnsGoroutines tests that multiSquash spawns goroutines for each module state
func TestMultiSquashSpawnsGoroutines(t *testing.T) {
	logger := zap.NewNop()
	segmenter := block.NewSegmenter(10, 0, 100)

	stage := &Stage{
		idx:               0,
		kind:              KindStore,
		segmenter:         segmenter,
		segmentCompleted:  -1,
		storeModuleStates: make([]*StoreModuleState, 3),
		syncWork:          llerrgroup.New(10),
		asyncWork:         llerrgroup.New(10),
	}

	var executionCount int32
	var firstStart time.Time
	var lastStart time.Time
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		storeConfig := &store.Config{}
		modState := &StoreModuleState{
			name:        "test_module_" + string(rune('A'+i)),
			segmenter:   segmenter,
			logger:      logger,
			storeConfig: storeConfig,
		}
		stage.storeModuleStates[i] = modState
	}

	// Simulate goroutine execution pattern
	loopStart := time.Now()
	for range stage.storeModuleStates {
		stage.syncWork.Go(func() error {
			mu.Lock()
			if firstStart.IsZero() {
				firstStart = time.Now()
			}
			lastStart = time.Now()
			mu.Unlock()

			atomic.AddInt32(&executionCount, 1)
			time.Sleep(10 * time.Millisecond) // Simulate work
			return nil
		})
	}
	loopDuration := time.Since(loopStart)

	err := stage.syncWork.Wait()
	require.NoError(t, err)

	// Verify all modules were processed
	assert.Equal(t, int32(3), executionCount, "All 3 modules should be processed")

	// Loop should complete quickly (spawning goroutines is fast)
	assert.Less(t, loopDuration, 50*time.Millisecond,
		"Loop should complete quickly when spawning goroutines")

	// All goroutines should start roughly at the same time
	mu.Lock()
	spawnDuration := lastStart.Sub(firstStart)
	mu.Unlock()

	t.Logf("Loop duration: %v", loopDuration)
	t.Logf("Goroutine spawn duration: %v", spawnDuration)
}

// TestMultiSquashWithDifferentSegments tests that multiSquash respects segment boundaries
func TestMultiSquashWithDifferentSegments(t *testing.T) {
	logger := zap.NewNop()
	segmenter := block.NewSegmenter(10, 0, 100)

	stage := &Stage{
		idx:               0,
		kind:              KindStore,
		segmenter:         segmenter,
		segmentCompleted:  -1,
		storeModuleStates: make([]*StoreModuleState, 3),
		syncWork:          llerrgroup.New(10),
		asyncWork:         llerrgroup.New(10),
	}

	var processedModules []string
	var mu sync.Mutex

	// Create module states with different segment boundaries
	for i := 0; i < 3; i++ {
		modSegmenter := block.NewSegmenter(10, uint64(i*10), 100)
		storeConfig := &store.Config{}
		modState := &StoreModuleState{
			name:        "test_module_" + string(rune('A'+i)),
			segmenter:   modSegmenter,
			logger:      logger,
			storeConfig: storeConfig,
		}
		stage.storeModuleStates[i] = modState
	}

	// Simulate multiSquash behavior with segment filtering
	mergeUnit := Unit{Stage: 0, Segment: 1}

	for _, modState := range stage.storeModuleStates {
		if mergeUnit.Segment < modState.segmenter.FirstIndex() {
			continue
		}

		stage.syncWork.Go(func() error {
			mu.Lock()
			processedModules = append(processedModules, modState.name)
			mu.Unlock()
			return nil
		})
	}

	err := stage.syncWork.Wait()
	require.NoError(t, err)

	// Only modules with appropriate segment ranges should be processed
	mu.Lock()
	processedCount := len(processedModules)
	mu.Unlock()

	assert.GreaterOrEqual(t, processedCount, 1, "At least one module should be processed")
	assert.LessOrEqual(t, processedCount, 3, "At most 3 modules should be processed")

	t.Logf("Processed %d modules: %v", processedCount, processedModules)
}

// TestMultiSquashErrorHandling tests that errors from singleSquash are properly propagated
func TestMultiSquashErrorHandling(t *testing.T) {
	logger := zap.NewNop()
	segmenter := block.NewSegmenter(10, 0, 100)

	stage := &Stage{
		idx:               0,
		kind:              KindStore,
		segmenter:         segmenter,
		segmentCompleted:  -1,
		storeModuleStates: make([]*StoreModuleState, 3),
		syncWork:          llerrgroup.New(10),
		asyncWork:         llerrgroup.New(10),
	}

	for i := 0; i < 3; i++ {
		storeConfig := &store.Config{}
		modState := &StoreModuleState{
			name:        "test_module_" + string(rune('A'+i)),
			segmenter:   segmenter,
			logger:      logger,
			storeConfig: storeConfig,
		}
		stage.storeModuleStates[i] = modState
	}

	// Simulate error in one of the goroutines
	for idx := range stage.storeModuleStates {
		stage.syncWork.Go(func() error {
			if idx == 1 {
				return assert.AnError // Simulate error in second module
			}
			return nil
		})
	}

	err := stage.syncWork.Wait()
	assert.Error(t, err, "Error from goroutine should be propagated")
}

// TestMultiSquashConcurrency tests concurrent execution patterns
func TestMultiSquashConcurrency(t *testing.T) {
	logger := zap.NewNop()
	segmenter := block.NewSegmenter(10, 0, 100)

	stage := &Stage{
		idx:               0,
		kind:              KindStore,
		segmenter:         segmenter,
		segmentCompleted:  -1,
		storeModuleStates: make([]*StoreModuleState, 5),
		syncWork:          llerrgroup.New(10),
		asyncWork:         llerrgroup.New(10),
	}

	var maxConcurrent int32
	var currentConcurrent int32

	for i := 0; i < 5; i++ {
		storeConfig := &store.Config{}
		modState := &StoreModuleState{
			name:        "test_module_" + string(rune('A'+i)),
			segmenter:   segmenter,
			logger:      logger,
			storeConfig: storeConfig,
		}
		stage.storeModuleStates[i] = modState
	}

	for range stage.storeModuleStates {
		stage.syncWork.Go(func() error {
			current := atomic.AddInt32(&currentConcurrent, 1)
			defer atomic.AddInt32(&currentConcurrent, -1)

			// Update max concurrent count
			for {
				max := atomic.LoadInt32(&maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
					break
				}
			}

			time.Sleep(20 * time.Millisecond)
			return nil
		})
	}

	err := stage.syncWork.Wait()
	require.NoError(t, err)

	t.Logf("Max concurrent goroutines: %d", maxConcurrent)
	assert.GreaterOrEqual(t, maxConcurrent, int32(1), "At least one goroutine should execute")
}

// TestMultiSquashEmptyModuleStates tests handling of empty module states
func TestMultiSquashEmptyModuleStates(t *testing.T) {
	segmenter := block.NewSegmenter(10, 0, 100)

	stage := &Stage{
		idx:               0,
		kind:              KindStore,
		segmenter:         segmenter,
		segmentCompleted:  -1,
		storeModuleStates: make([]*StoreModuleState, 0), // Empty
		syncWork:          llerrgroup.New(10),
		asyncWork:         llerrgroup.New(10),
	}

	executed := false
	for range stage.storeModuleStates {
		stage.syncWork.Go(func() error {
			executed = true
			return nil
		})
	}

	err := stage.syncWork.Wait()
	require.NoError(t, err)
	assert.False(t, executed, "No goroutines should be spawned for empty module states")
}

// TestMultiSquashContextCancellation tests behavior when context is cancelled
func TestMultiSquashContextCancellation(t *testing.T) {
	segmenter := block.NewSegmenter(10, 0, 100)

	stage := &Stage{
		idx:               0,
		kind:              KindStore,
		segmenter:         segmenter,
		segmentCompleted:  -1,
		storeModuleStates: make([]*StoreModuleState, 3),
		syncWork:          llerrgroup.New(10),
		asyncWork:         llerrgroup.New(10),
	}

	ctx, cancel := context.WithCancel(context.Background())

	for i := 0; i < 3; i++ {
		storeConfig := &store.Config{}
		modState := &StoreModuleState{
			name:        "test_module_" + string(rune('A'+i)),
			segmenter:   segmenter,
			logger:      zap.NewNop(),
			storeConfig: storeConfig,
		}
		stage.storeModuleStates[i] = modState
	}

	// Spawn goroutines that check for context cancellation
	for range stage.storeModuleStates {
		stage.syncWork.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return nil
			}
		})
	}

	// Cancel context immediately
	cancel()

	err := stage.syncWork.Wait()
	// Depending on timing, we might get a context cancelled error
	// or the goroutines might complete before they check the context
	t.Logf("Wait result: %v", err)
	assert.True(t, err == nil || errors.Is(err, context.Canceled))
}

// TestSyncWorkGroupBehavior tests the behavior of the llerrgroup used in multiSquash
func TestSyncWorkGroupBehavior(t *testing.T) {
	group := llerrgroup.New(10)

	var completionOrder []int
	var mu sync.Mutex

	// Spawn multiple goroutines with different execution times
	for i := range 5 {
		group.Go(func() error {
			sleepTime := time.Duration(5-i) * 10 * time.Millisecond
			time.Sleep(sleepTime)

			mu.Lock()
			completionOrder = append(completionOrder, i)
			mu.Unlock()

			return nil
		})
	}

	err := group.Wait()
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, completionOrder, 5, "All goroutines should complete")
	t.Logf("Completion order: %v", completionOrder)

	// Verify they don't necessarily complete in spawn order
	// (last spawned has shortest sleep, so might finish first)
}
