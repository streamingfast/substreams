package exec

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStepableObject implements bstream.Stepable for testing
type testStepableObject struct {
	step bstream.StepType
}

func (t *testStepableObject) Step() bstream.StepType {
	return t.step
}

func (t *testStepableObject) FinalBlockHeight() uint64 {
	return 0
}

func (t *testStepableObject) ReorgJunctionBlock() bstream.BlockRef {
	return nil
}

func TestUndoManager_BasicFunctionality(t *testing.T) {
	um := NewUndoManager(0)

	// Test empty manager
	assert.False(t, um.Contains("nonexistent"))

	// Test subscription to non-existent block
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subCtx, unsubscribe := um.Subscribe(ctx, "block1")
	defer unsubscribe()

	// Context should not be cancelled yet
	select {
	case <-subCtx.Done():
		t.Fatal("Context should not be cancelled")
	default:
	}

	// Process an undo for the block
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepUndo}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)

	// Now the context should be cancelled
	select {
	case <-subCtx.Done():
		assert.Equal(t, ErrBlockUndo, context.Cause(subCtx))
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context should have been cancelled")
	}

	// The block should now be in previousUndoneBlocks
	assert.True(t, um.Contains("block1"))
}

func TestUndoManager_SubscribeToAlreadyUndonePreviousBlock(t *testing.T) {
	um := NewUndoManager(0)

	// First, undo a block
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepUndo}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)

	// Now subscribe to the already undone block
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subCtx, unsubscribe := um.Subscribe(ctx, "block1")
	defer unsubscribe()

	// Context should be immediately cancelled
	select {
	case <-subCtx.Done():
		assert.Equal(t, ErrBlockUndo, context.Cause(subCtx))
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Context should have been immediately cancelled")
	}
}

func TestUndoManager_ProcessNewBlock(t *testing.T) {
	um := NewUndoManager(0)

	// First undo a block
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepUndo}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)
	assert.True(t, um.Contains("block1"))

	// Now process the same block as NEW (this should trigger a warning but not fail)
	stepableObj = &testStepableObject{step: bstream.StepNew}
	err = um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)

	// Block should still be in undone blocks
	assert.True(t, um.Contains("block1"))
}

func TestUndoManager_ProcessNonStepableObject(t *testing.T) {
	um := NewUndoManager(0)

	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}

	// Process with non-stepable object
	err := um.ProcessBlock(block, "not stepable")
	require.NoError(t, err)

	// Nothing should happen
	assert.False(t, um.Contains("block1"))
}

func TestUndoManager_MaxUndoneBlocksSize(t *testing.T) {
	um := NewUndoManager(0)
	originalMaxSize := MaxUndoneBlocksSize
	MaxUndoneBlocksSize = 3 // Set small size for testing
	defer func() { MaxUndoneBlocksSize = originalMaxSize }()

	// Add blocks beyond the max size
	for i := 0; i < 5; i++ {
		block := &pbbstream.Block{
			Id:     fmt.Sprintf("block%d", i),
			Number: uint64(100 + i),
		}
		stepableObj := &testStepableObject{step: bstream.StepUndo}

		err := um.ProcessBlock(block, stepableObj)
		require.NoError(t, err)
	}

	// Should only have MaxUndoneBlocksSize blocks
	um.Lock()
	assert.Equal(t, MaxUndoneBlocksSize, len(um.previousUndoneBlocks))

	// Check which blocks are present without calling Contains (which would deadlock)
	_, has0 := um.previousUndoneBlocks["block0"]
	_, has1 := um.previousUndoneBlocks["block1"]
	_, has2 := um.previousUndoneBlocks["block2"]
	_, has3 := um.previousUndoneBlocks["block3"]
	_, has4 := um.previousUndoneBlocks["block4"]
	um.Unlock()

	// The oldest blocks should have been removed (block0 and block1)
	assert.False(t, has0)
	assert.False(t, has1)
	assert.True(t, has2)
	assert.True(t, has3)
	assert.True(t, has4)
}

func TestUndoManager_MultipleSubscribersToSameBlock(t *testing.T) {
	um := NewUndoManager(0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe multiple times to the same block
	subCtx1, unsubscribe1 := um.Subscribe(ctx, "block1")
	defer unsubscribe1()

	subCtx2, unsubscribe2 := um.Subscribe(ctx, "block1")
	defer unsubscribe2()

	subCtx3, unsubscribe3 := um.Subscribe(ctx, "block1")
	defer unsubscribe3()

	// Process undo for the block
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepUndo}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)

	// All contexts should be cancelled
	contexts := []context.Context{subCtx1, subCtx2, subCtx3}
	for i, subCtx := range contexts {
		select {
		case <-subCtx.Done():
			assert.Equal(t, ErrBlockUndo, context.Cause(subCtx), "Context %d should be cancelled with ErrBlockUndo", i)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Context %d should have been cancelled", i)
		}
	}
}

func TestUndoManager_UnsubscribeBeforeUndo(t *testing.T) {
	um := NewUndoManager(0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subCtx, unsubscribe := um.Subscribe(ctx, "block1")

	// Unsubscribe immediately
	unsubscribe()

	// Process undo for the block
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepUndo}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)

	// Context should not be cancelled since we unsubscribed
	select {
	case <-subCtx.Done():
		// If it's cancelled, it should not be due to ErrBlockUndo
		assert.NotEqual(t, ErrBlockUndo, context.Cause(subCtx), "Context should not be cancelled with ErrBlockUndo after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// Expected behavior - context not cancelled
	}
}

func TestUndoManager_ConcurrentSubscribeAndProcessBlock(t *testing.T) {
	um := NewUndoManager(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const numSubscribers = 100
	const numBlocks = 50

	var wg sync.WaitGroup
	var mu sync.Mutex
	cancelledContexts := make(map[string]int)

	// Start multiple subscribers concurrently
	for i := 0; i < numSubscribers; i++ {
		wg.Add(1)
		go func(subscriberID int) {
			defer wg.Done()

			blockID := fmt.Sprintf("block%d", subscriberID%numBlocks)
			subCtx, unsubscribe := um.Subscribe(ctx, blockID)
			defer unsubscribe()

			// Wait for context to be done or timeout
			select {
			case <-subCtx.Done():
				if context.Cause(subCtx) == ErrBlockUndo {
					mu.Lock()
					cancelledContexts[blockID]++
					mu.Unlock()
				}
			case <-time.After(2 * time.Second):
				// Timeout is fine, not all blocks will be undone
			}
		}(i)
	}

	// Start processing blocks linearly (normal use case)
	for blockNum := 0; blockNum < numBlocks; blockNum++ {
		// Add some delay to allow subscribers to register
		time.Sleep(time.Duration(blockNum) * time.Millisecond)

		block := &pbbstream.Block{
			Id:     fmt.Sprintf("block%d", blockNum),
			Number: uint64(100 + blockNum),
		}
		stepableObj := &testStepableObject{step: bstream.StepUndo}

		err := um.ProcessBlock(block, stepableObj)
		assert.NoError(t, err)
	}

	wg.Wait()

	// Verify no deadlocks occurred and some contexts were cancelled
	mu.Lock()
	assert.True(t, len(cancelledContexts) > 0, "Some contexts should have been cancelled")
	mu.Unlock()
}

func TestUndoManager_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	um := NewUndoManager(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const numWorkers = 50
	const iterations = 100

	var wg sync.WaitGroup

	// Workers that subscribe and unsubscribe rapidly
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				blockID := fmt.Sprintf("block%d", (workerID*iterations+j)%10)
				subCtx, unsubscribe := um.Subscribe(ctx, blockID)

				// Sometimes unsubscribe immediately, sometimes wait a bit
				if j%2 == 0 {
					unsubscribe()
				} else {
					go func() {
						time.Sleep(time.Millisecond)
						unsubscribe()
					}()
				}

				// Consume the context to prevent goroutine leaks
				go func(ctx context.Context) {
					<-ctx.Done()
				}(subCtx)
			}
		}(i)
	}

	// Worker that processes blocks
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < iterations; i++ {
			blockID := fmt.Sprintf("block%d", i%10)
			block := &pbbstream.Block{
				Id:     blockID,
				Number: uint64(100 + i),
			}
			stepableObj := &testStepableObject{step: bstream.StepUndo}

			err := um.ProcessBlock(block, stepableObj)
			assert.NoError(t, err)

			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Test should complete without deadlock
}

func TestUndoManager_UniqueIDGeneration(t *testing.T) {
	// Test that unique IDs are generated correctly
	const numIDs = 1000
	ids := make(map[uint64]bool)

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Generate IDs concurrently
	for i := 0; i < numIDs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := nextUniqueID()

			mu.Lock()
			assert.False(t, ids[id], "ID %d should be unique", id)
			ids[id] = true
			mu.Unlock()
		}()
	}

	wg.Wait()
	assert.Equal(t, numIDs, len(ids), "All IDs should be unique")
}

func TestUndoManager_ContextCancelPropagation(t *testing.T) {
	um := NewUndoManager(0)

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	subCtx, unsubscribe := um.Subscribe(ctx, "block1")
	defer unsubscribe()

	// Cancel the parent context
	cancel()

	// Child context should be cancelled
	select {
	case <-subCtx.Done():
		// This should happen, but the cause should be from parent cancellation, not ErrBlockUndo
		assert.NotEqual(t, ErrBlockUndo, context.Cause(subCtx))
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Context should have been cancelled")
	}
}

func TestUndoManager_MemoryLeakPrevention(t *testing.T) {
	um := NewUndoManager(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to a block
	_, unsubscribe := um.Subscribe(ctx, "block1")

	// Check that subscription exists
	um.Lock()
	assert.Len(t, um.activeSubscriptions["block1"], 1)
	um.Unlock()

	// Unsubscribe
	unsubscribe()

	// Check that subscription is removed
	um.Lock()
	assert.Len(t, um.activeSubscriptions["block1"], 0)
	um.Unlock()
}
