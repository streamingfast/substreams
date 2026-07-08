package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/storage/store/marshaller"
)

// TestSnapshotStreamRoundTrip serializes a store through the lazy snapshot
// path (Snapshot → MarshalStreamSnapshot) and loads it back through the
// streaming unmarshal path, on both backends. The key count exceeds
// snapshotBatchMaxKeys so the mmap iterator exercises multiple refill
// transactions and the Seek-based batch resumption.
func TestSnapshotStreamRoundTrip(t *testing.T) {
	const numKeys = snapshotBatchMaxKeys*2 + 500

	for _, backend := range []string{"mmap", "memory"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("SUBSTREAMS_STORE_BACKEND", backend)

			src := createTestStore(t, "snap_roundtrip_src", 0, "").kvImpl
			defer src.Close()

			kv := make(map[string][]byte, numKeys)
			for i := 0; i < numKeys; i++ {
				kv[fmt.Sprintf("key:%08d", i)] = []byte(fmt.Sprintf("value_%d", i))
			}
			require.NoError(t, src.BatchSet(kv))

			deletePrefixes := []string{"gone:", "also_gone:"}

			snap, err := src.Snapshot()
			require.NoError(t, err)
			sm := &marshaller.VTproto{}
			reader := sm.MarshalStreamSnapshot(snap, deletePrefixes)
			serialized, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())

			dst := createTestStore(t, "snap_roundtrip_dst", 0, "").kvImpl
			defer dst.Close()

			var gotPrefixes []string
			size, err := unmarshalIterInto(context.Background(), dst, sm, bytes.NewReader(serialized), func(dp []string) {
				gotPrefixes = dp
			})
			require.NoError(t, err)

			assert.Equal(t, numKeys, dst.KeyCount())
			assert.Equal(t, deletePrefixes, gotPrefixes)
			assert.NotZero(t, size)

			for i := 0; i < numKeys; i += numKeys / 100 {
				k := fmt.Sprintf("key:%08d", i)
				v, found := dst.Get(k)
				require.True(t, found, "missing key %q", k)
				assert.Equal(t, []byte(fmt.Sprintf("value_%d", i)), v)
			}
		})
	}
}

// TestMmapSnapshotBlocksWrites verifies the snapshot write-gate: while a
// snapshot is open, Set blocks; it proceeds as soon as the snapshot closes.
func TestMmapSnapshotBlocksWrites(t *testing.T) {
	t.Setenv("SUBSTREAMS_STORE_BACKEND", "mmap")

	impl := createTestStore(t, "snap_write_gate", 0, "").kvImpl
	defer impl.Close()

	require.NoError(t, impl.Set("existing", []byte("x")))

	snap, err := impl.Snapshot()
	require.NoError(t, err)

	wrote := make(chan error, 1)
	go func() {
		wrote <- impl.Set("blocked", []byte("y"))
	}()

	select {
	case <-wrote:
		t.Fatal("Set completed while snapshot was open")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, snap.Close())

	select {
	case err := <-wrote:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Set still blocked after snapshot close")
	}

	v, found := impl.Get("blocked")
	require.True(t, found)
	assert.Equal(t, []byte("y"), v)
}

// TestMmapSnapshotIterLifecycle covers Close idempotency and use-after-close.
func TestMmapSnapshotIterLifecycle(t *testing.T) {
	t.Setenv("SUBSTREAMS_STORE_BACKEND", "mmap")

	impl := createTestStore(t, "snap_lifecycle", 0, "").kvImpl
	defer impl.Close()

	require.NoError(t, impl.Set("a", []byte("1")))

	snap, err := impl.Snapshot()
	require.NoError(t, err)

	k, v, ok, err := snap.Next()
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "a", k)
	assert.Equal(t, []byte("1"), v)

	require.NoError(t, snap.Close())
	require.NoError(t, snap.Close()) // idempotent

	_, _, _, err = snap.Next()
	require.Error(t, err)
}
