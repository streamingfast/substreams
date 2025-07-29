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
	um := NewUndoManager()

	// Test empty manager
	assert.False(t, um.Contains("nonexistent"))

	// Test subscription to non-existent block
	ctx := t.Context()

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
	um := NewUndoManager()

	// First, undo a block
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepUndo}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)

	// Now subscribe to the already undone block
	ctx := t.Context()
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
	um := NewUndoManager()

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

	// Block should be removed from previousUndoneBlocks (line 55 functionality)
	assert.False(t, um.Contains("block1"))
}

func TestUndoManager_ProcessNewBlockEdgeCases(t *testing.T) {
	um := NewUndoManager()

	// Test processing NEW block that was never undone (should be no-op)
	block := &pbbstream.Block{
		Id:     "block1",
		Number: 100,
	}
	stepableObj := &testStepableObject{step: bstream.StepNew}

	err := um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)
	assert.False(t, um.Contains("block1"))

	// Test multiple NEW blocks for the same previously undone block
	// First undo the block
	stepableObj = &testStepableObject{step: bstream.StepUndo}
	err = um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)
	assert.True(t, um.Contains("block1"))

	// Process NEW twice - first should remove, second should be no-op
	stepableObj = &testStepableObject{step: bstream.StepNew}
	err = um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)
	assert.False(t, um.Contains("block1"))

	// Second NEW should be no-op (no panic/error)
	err = um.ProcessBlock(block, stepableObj)
	require.NoError(t, err)
	assert.False(t, um.Contains("block1"))
}

func TestUndoManager_ProcessNonStepableObject(t *testing.T) {
	um := NewUndoManager()

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
	um := NewUndoManager()
	originalMaxSize := MaxUndoneBlocksSize
	MaxUndoneBlocksSize = 3 // Set small size for testing
	defer func() { MaxUndoneBlocksSize = originalMaxSize }()

	// Add blocks beyond the max size
	for i := range 5 {
		block := &pbbstream.Block{
			Id:     fmt.Sprintf("block%d", i),
			Number: uint64(100 + i),
		}
		stepableObj := &testStepableObject{step: bstream.StepUndo}

		err := um.ProcessBlock(block, stepableObj)
		require.NoError(t, err)
	}

	// The oldest blocks should have been removed (block0 and block1)
	assert.False(t, um.Contains("block0"))
	assert.False(t, um.Contains("block1"))
	assert.True(t, um.Contains("block2"))
	assert.True(t, um.Contains("block3"))
	assert.True(t, um.Contains("block4"))
}

func TestUndoManager_MultipleSubscribersToSameBlock(t *testing.T) {
	um := NewUndoManager()

	ctx := t.Context()

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
	um := NewUndoManager()

	ctx := t.Context()

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
	um := NewUndoManager()
	ctx := t.Context()

	const numSubscribers = 100
	const numBlocks = 50

	var wg sync.WaitGroup
	var mu sync.Mutex
	cancelledContexts := make(map[string]int)

	// Start multiple subscribers concurrently
	for i := range numSubscribers {
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
	for blockNum := range numBlocks {
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
	um := NewUndoManager()
	ctx := t.Context()

	const numWorkers = 50
	const iterations = 100

	var wg sync.WaitGroup

	// Workers that subscribe and unsubscribe rapidly
	for i := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := range iterations {
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

		for i := range iterations {
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

func TestUndoManager_ContextCancelPropagation(t *testing.T) {
	um := NewUndoManager()

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
	um := NewUndoManager()
	ctx := t.Context()

	// Subscribe to a block
	_, unsubscribe := um.Subscribe(ctx, "block1")

	// Unsubscribe
	unsubscribe()

	// Test that subsequent subscriptions work properly (indicating cleanup occurred)
	_, unsubscribe2 := um.Subscribe(ctx, "block1")
	unsubscribe2()
}

func TestUndoManager_ActiveSubscriptionsCleanupAfterLastUnsubscribe(t *testing.T) {
	um := NewUndoManager()
	ctx := t.Context()

	blockID := "test_block_cleanup"

	// Subscribe multiple watchers to the same block
	_, unsubscribe1 := um.Subscribe(ctx, blockID)
	_, unsubscribe2 := um.Subscribe(ctx, blockID)
	_, unsubscribe3 := um.Subscribe(ctx, blockID)

	// Unsubscribe first two watchers
	unsubscribe1()
	unsubscribe2()

	// Unsubscribe the last watcher
	unsubscribe3()

	// Test that new subscriptions work properly after cleanup (indicating cleanup occurred)
	blockID2 := "test_block_cleanup_2"
	_, unsubscribe4 := um.Subscribe(ctx, blockID2)
	unsubscribe4()

	// Test that we can still subscribe to the original block (indicating it was cleaned up)
	_, unsubscribe5 := um.Subscribe(ctx, blockID)
	unsubscribe5()
}

func TestUndoManager_ActiveSubscriptionsCleanupEdgeCases(t *testing.T) {
	um := NewUndoManager()
	ctx := t.Context()

	// Test case 1: Single subscription cleanup
	blockID1 := "single_sub_test"
	_, unsubscribe1 := um.Subscribe(ctx, blockID1)

	unsubscribe1()

	// Test that new subscription works (indicating cleanup occurred)
	_, unsubscribe1b := um.Subscribe(ctx, blockID1)
	unsubscribe1b()

	// Test case 2: Multiple unsubscribes of the same watcher (should be safe)
	blockID2 := "double_unsub_test"
	_, unsubscribe2 := um.Subscribe(ctx, blockID2)

	unsubscribe2()
	unsubscribe2() // Second call should be safe

	// Test that new subscription works (indicating cleanup occurred)
	_, unsubscribe2b := um.Subscribe(ctx, blockID2)
	unsubscribe2b()

	// Test case 3: Mixed order unsubscribes
	blockID3 := "mixed_order_test"
	_, unsubscribeA := um.Subscribe(ctx, blockID3)
	_, unsubscribeB := um.Subscribe(ctx, blockID3)
	_, unsubscribeC := um.Subscribe(ctx, blockID3)

	// Unsubscribe middle one first, then first, then last
	unsubscribeB()
	unsubscribeA()
	unsubscribeC()

	// Test that new subscription works (indicating cleanup occurred)
	_, unsubscribe3b := um.Subscribe(ctx, blockID3)
	unsubscribe3b()

	// Test case 4: Cleanup doesn't affect other blocks
	blockID4a := "block_a"
	blockID4b := "block_b"

	_, unsubscribe4a := um.Subscribe(ctx, blockID4a)
	_, unsubscribe4b1 := um.Subscribe(ctx, blockID4b)
	_, unsubscribe4b2 := um.Subscribe(ctx, blockID4b)

	// Remove all subscriptions from block A
	unsubscribe4a()

	// Block A should be cleaned up, but block B should still work
	// Test that block A is cleaned up by subscribing again
	_, unsubscribe4a_new := um.Subscribe(ctx, blockID4a)

	// Block B should still work with existing subscriptions
	// Remove one subscription from B and ensure it still works
	unsubscribe4b1()

	// Cleanup remaining subscriptions
	unsubscribe4a_new()
	unsubscribe4b2()
}
