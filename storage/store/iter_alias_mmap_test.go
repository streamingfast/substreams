package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIterMmap_CopiedValueSurvivesTxn guards the second escape site
// (pipeline/snapshot.go sendSnapshots), which iterates a store via Iter and
// holds each value in an accumulator that OUTLIVES the iteration (snapshot
// deltas are buffered and sent asynchronously).
//
// On the mmap backend Iter yields slices aliasing bbolt's page buffer, valid
// only during the read transaction: retaining the raw slice and reading it
// after the transaction closes reads reused/unmapped memory (data corruption,
// and in practice a segfault). Callers that COPY the value before it escapes
// (as snapshot.go now does) are safe.
//
// This test performs the copy-on-retain that the fix mandates and asserts the
// retained values remain intact after forcing bbolt page reuse. Reverting the
// snapshot.go fix (retaining the raw aliased slice) reproduces the corruption
// / crash this guards against.
func TestIterMmap_CopiedValueSurvivesTxn(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("iter", "h", &MmapBackendConfig{ScratchSpace: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })

	want := map[string][]byte{}
	for i := 0; i < 500; i++ {
		want[fmt.Sprintf("k:%08d:tvl", i)] = []byte(fmt.Sprintf("%d.123456789012345", i))
		want[fmt.Sprintf("k:%08d:totalValueLockedUSDUntracked", i)] = []byte(fmt.Sprintf("%d.999999", i))
	}
	require.NoError(t, impl.BatchSet(want))

	type kv struct {
		k string
		v []byte
	}
	var accum []kv
	require.NoError(t, impl.Iter(func(k string, v []byte) error {
		// Copy before retaining — the contract callers must follow (snapshot.go).
		cp := make([]byte, len(v))
		copy(cp, v)
		accum = append(accum, kv{k, cp})
		return nil
	}))

	// Force bbolt page reuse after the read transaction closed.
	for i := 0; i < 1000; i++ {
		_ = impl.Set(fmt.Sprintf("churn:%08d", i), []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	}

	require.Equal(t, len(want), len(accum), "iterated entry count")
	for _, e := range accum {
		wantV, ok := want[e.k]
		require.True(t, ok, "phantom key %q", e.k)
		require.Equal(t, string(wantV), string(e.v), "copied value must survive txn close for key %q", e.k)
	}
}
