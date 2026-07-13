package store

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMemorySnapshot_StableUnderConcurrentWrites pins the fix for the
// memory-vs-mmap snapshot asymmetry: memory Snapshot() must capture a stable
// view at call time, not read the live map per Next(). With the lazy streaming
// marshal a snapshot's read window spans the whole upload; if it read the live
// map while another goroutine wrote it, Go would raise a fatal, unrecoverable
// "concurrent map read and map write" panic (which -race also flags). Copying
// the entries up front gives memory the same "frozen during save" safety mmap
// has via its snapMu write-gate.
//
// This test iterates a memory snapshot to completion while a writer mutates the
// underlying map, and asserts the snapshot returns exactly the entries present
// at Snapshot() time — unaffected by concurrent writes.
func TestMemorySnapshot_StableUnderConcurrentWrites(t *testing.T) {
	m := newMemoryKVImpl()

	seed := map[string][]byte{}
	for i := 0; i < 2000; i++ {
		seed[fmt.Sprintf("seed-%06d", i)] = []byte(fmt.Sprintf("v-%d", i))
	}
	_, err := m.Load(mapToIter(seed))
	require.NoError(t, err)

	snap, err := m.Snapshot()
	require.NoError(t, err)
	defer snap.Close()

	// Writer mutates the live map concurrently with snapshot iteration.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				_ = m.Set(fmt.Sprintf("live-%06d", i), []byte("x"))
				i++
			}
		}
	}()

	// Drain the snapshot: must see exactly the seeded keys, no "live-*" keys,
	// no missing seed keys, and, critically, no fatal concurrent-map panic.
	got := map[string]struct{}{}
	for {
		k, _, ok, err := snap.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		got[k] = struct{}{}
	}

	close(stop)
	wg.Wait()

	require.Len(t, got, len(seed), "snapshot must contain exactly the entries present at Snapshot() time")
	for k := range seed {
		_, ok := got[k]
		require.True(t, ok, "seed key %q missing from snapshot", k)
	}
	for k := range got {
		_, isSeed := seed[k]
		require.True(t, isSeed, "snapshot leaked a concurrently-written key %q", k)
	}
}
