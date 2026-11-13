package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMmapLongLivedSnapshotWritesProceedConcurrently is the mirror image of
// TestMmapSnapshotBlocksWrites: with LongLivedSnapshot opted in and enough mmap
// headroom, a Set must not stall while a snapshot is open, and bbolt's MVCC must
// keep that snapshot from ever observing it.
func TestMmapLongLivedSnapshotWritesProceedConcurrently(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("longtx_concurrent", "h", &MmapBackendConfig{
		ScratchSpace:      t.TempDir(),
		InitialMmapSize:   4 << 20, // 4 MiB — generous for the handful of keys below
		LongLivedSnapshot: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })

	require.NoError(t, impl.Set("existing", []byte("x")))

	snap, err := impl.Snapshot()
	require.NoError(t, err)

	wrote := make(chan error, 1)
	go func() {
		wrote <- impl.Set("written-during-snapshot", []byte("y"))
	}()

	select {
	case err := <-wrote:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "Set stalled while a long-lived snapshot was open")
	}

	// MVCC isolation: the write above landed after the read transaction started, so
	// the open snapshot must not see it even though the write itself already succeeded.
	seen := map[string]bool{}
	for {
		k, _, ok, err := snap.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		seen[k] = true
	}
	assert.True(t, seen["existing"])
	assert.False(t, seen["written-during-snapshot"], "long-lived snapshot observed a write made after it opened")
	require.NoError(t, snap.Close())

	v, found := impl.Get("written-during-snapshot")
	require.True(t, found)
	assert.Equal(t, []byte("y"), v)
}

// TestMmapLongLivedSnapshotFallsBackWhenHeadroomThin forces the store's live size past
// longLivedSnapshotHeadroomFraction of a deliberately tiny reservation, then asserts
// Snapshot falls back to the exclusive write-gate instead of risking the remap race a
// long-lived read transaction would be exposed to (see Snapshot's doc comment): the
// fallback is observable as the same write-blocks-until-close behavior as the
// LongLivedSnapshot-disabled path.
func TestMmapLongLivedSnapshotFallsBackWhenHeadroomThin(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("longtx_fallback", "h", &MmapBackendConfig{
		ScratchSpace:      t.TempDir(),
		InitialMmapSize:   64 << 10, // 64 KiB — the data below outgrows this many times over
		LongLivedSnapshot: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })

	kv := map[string][]byte{}
	for i := range 2000 {
		kv[fmt.Sprintf("key:%06d", i)] = fmt.Appendf(nil, "value-payload-%d-padding-padding-padding", i)
	}
	_, err = impl.Load(mapToIter(kv))
	require.NoError(t, err)

	snap, err := impl.Snapshot()
	require.NoError(t, err)

	wrote := make(chan error, 1)
	go func() {
		wrote <- impl.Set("blocked", []byte("y"))
	}()

	select {
	case <-wrote:
		require.Fail(t, "Set completed while the fallback exclusive snapshot was open")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, snap.Close())

	select {
	case err := <-wrote:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "Set still blocked after snapshot close")
	}
}

// TestMmapLongLivedSnapshotCloseWaits ensures Close blocks on an in-flight long-lived
// snapshot, the same guarantee the exclusive path gets from snapMu — a leaked or
// unusually long upload must never let the backing file get removed from under it.
func TestMmapLongLivedSnapshotCloseWaits(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("longtx_close_waits", "h", &MmapBackendConfig{
		ScratchSpace:      t.TempDir(),
		InitialMmapSize:   4 << 20,
		LongLivedSnapshot: true,
	})
	require.NoError(t, err)

	require.NoError(t, impl.Set("a", []byte("1")))
	snap, err := impl.Snapshot()
	require.NoError(t, err)

	closed := make(chan error, 1)
	go func() { closed <- impl.Close() }()

	select {
	case <-closed:
		require.Fail(t, "Close returned while a long-lived snapshot was open")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, snap.Close())

	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "Close did not return once the snapshot closed")
	}
}
