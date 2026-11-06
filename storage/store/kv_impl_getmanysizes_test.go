package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestKVImpls returns one instance of every KVImpl backend, each loaded with
// kv, so a single test body can assert identical behaviour across backends.
func newTestKVImpls(t *testing.T, kv map[string][]byte) map[string]KVImpl {
	t.Helper()

	mem := newMemoryKVImpl()
	_, err := mem.Load(mapToIter(kv))
	require.NoError(t, err)

	mm, err := newMmapKVImplWithConfig("t", "h", &MmapBackendConfig{ScratchSpace: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { mm.Close() })
	_, err = mm.Load(mapToIter(kv))
	require.NoError(t, err)

	return map[string]KVImpl{"memory": mem, "mmap": mm}
}

// TestGetManySizes_MatchesGetMany pins the core invariant the merge accounting
// relies on: GetManySizes reports exactly the presence set and value lengths
// that GetMany would, on every backend. If these ever diverge, SET /
// SET_IF_NOT_EXISTS merges would mis-account totalSizeBytes or (for SIONE)
// wrongly overwrite existing keys.
func TestGetManySizes_MatchesGetMany(t *testing.T) {
	kv := map[string][]byte{}
	for i := 0; i < 500; i++ {
		kv[fmt.Sprintf("key:%04d", i)] = []byte(fmt.Sprintf("value-%d", i))
	}

	// query a mix of present and absent keys
	var keys []string
	for i := 0; i < 500; i += 2 {
		keys = append(keys, fmt.Sprintf("key:%04d", i)) // present
	}
	keys = append(keys, "absent-1", "absent-2", "key:0001")

	for name, impl := range newTestKVImpls(t, kv) {
		t.Run(name, func(t *testing.T) {
			values, err := impl.GetMany(keys)
			require.NoError(t, err)

			sizes, err := impl.GetManySizes(keys)
			require.NoError(t, err)

			require.Equal(t, len(values), len(sizes), "presence set must match GetMany")
			for k, v := range values {
				sz, ok := sizes[k]
				require.True(t, ok, "key %q present in GetMany but missing in GetManySizes", k)
				require.Equal(t, len(v), sz, "size mismatch for key %q", k)
			}
			for _, absent := range []string{"absent-1", "absent-2"} {
				_, ok := sizes[absent]
				require.False(t, ok, "absent key %q must not appear in GetManySizes", absent)
			}
		})
	}
}
