package store

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

// fullForPolicy builds an empty FullKV on the named backend for size-accounting
// tests. Both backends run the exact same accounting code; asserting the
// absolute totalSizeBytes (not just memory-vs-mmap parity) pins the arithmetic
// of the GetManySizes path used by SET / SET_IF_NOT_EXISTS.
func fullForPolicy(t *testing.T, backend string, kv map[string][]byte, up pbsubstreams.Module_KindStore_UpdatePolicy, vt string) *FullKV {
	t.Helper()
	switch backend {
	case "memory":
		return newStore(cloneKV(kv), up, vt)
	case "mmap":
		return mmapFull(t, cloneKV(kv), up, vt)
	default:
		t.Fatalf("unknown backend %q", backend)
		return nil
	}
}

func backends() []string { return []string{"memory", "mmap"} }

// TestMergeSet_SizeAccounting exercises the SET merge path, which now reads only
// value lengths (GetManySizes) instead of the full previous values (GetMany).
// It checks that new-key and overwrite accounting both stay exact.
func TestMergeSet_SizeAccounting(t *testing.T) {
	up := pbsubstreams.Module_KindStore_UPDATE_POLICY_SET
	vt := "string"

	for _, backend := range backends() {
		t.Run(backend, func(t *testing.T) {
			full := fullForPolicy(t, backend, map[string][]byte{}, up, vt)

			// First merge: two brand-new keys.
			// size = (len("aa")+len("1")) + (len("bb")+len("22")) = 3 + 4 = 7
			require.NoError(t, full.Merge(newPartialStore(map[string][]byte{
				"aa": []byte("1"),
				"bb": []byte("22"),
			}, up, vt, nil)))
			require.Equal(t, uint64(7), full.SizeBytes(), "size after inserting two new keys")

			// Second merge: overwrite "aa" (old len 1 -> new len 3, delta +2) and
			// insert new key "cc" (len("cc")+len("3") = 3). total = 7 + 2 + 3 = 12
			require.NoError(t, full.Merge(newPartialStore(map[string][]byte{
				"aa": []byte("999"),
				"cc": []byte("3"),
			}, up, vt, nil)))
			require.Equal(t, uint64(12), full.SizeBytes(), "size after overwrite + new key")

			assertGet(t, full, "aa", "999")
			assertGet(t, full, "bb", "22")
			assertGet(t, full, "cc", "3")
			require.Equal(t, 3, full.kvImpl.KeyCount())
		})
	}
}

// TestMergeSetIfNotExists checks the SET_IF_NOT_EXISTS path: existing keys keep
// their value and are NOT re-accounted, only genuinely new keys are written.
// Correct behaviour hinges on GetManySizes reporting prior existence.
func TestMergeSetIfNotExists(t *testing.T) {
	up := pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS
	vt := "string"

	for _, backend := range backends() {
		t.Run(backend, func(t *testing.T) {
			full := fullForPolicy(t, backend, map[string][]byte{"exist": []byte("old")}, up, vt)
			// helper does not seed totalSizeBytes from the pre-loaded key, so the
			// counter starts at 0 and we assert only the merge-induced delta.

			require.NoError(t, full.Merge(newPartialStore(map[string][]byte{
				"exist": []byte("NEW-should-be-ignored"),
				"fresh": []byte("x"),
			}, up, vt, nil)))

			// "exist" is untouched; only "fresh" is written and accounted:
			// size delta = len("fresh") + len("x") = 6
			assertGet(t, full, "exist", "old")
			assertGet(t, full, "fresh", "x")
			require.Equal(t, uint64(6), full.SizeBytes(), "only the new key should be accounted")
			require.Equal(t, 2, full.kvImpl.KeyCount())
		})
	}
}

func assertGet(t *testing.T, s *FullKV, key, want string) {
	t.Helper()
	v, found := s.kvImpl.Get(key)
	require.True(t, found, "key %q must exist", key)
	require.Equal(t, want, string(v), "value for key %q", key)
}
