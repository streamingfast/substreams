package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// withMergeChunkSize temporarily shrinks the Merge window so tests can cross
// chunk boundaries with small inputs.
func withMergeChunkSize(n int, fn func()) {
	orig := mergeChunkSize
	mergeChunkSize = n
	defer func() { mergeChunkSize = orig }()
	fn()
}

// runMerge merges partialKV into prevKV under the given policy with mergeChunkSize
// forced to chunk, returning the resulting KV (as strings) and the store's
// totalSizeBytes accounting.
func runMerge(t *testing.T, prevKV, partialKV map[string][]byte, policy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, chunk int) (map[string]string, uint64) {
	t.Helper()
	out := map[string]string{}
	var size uint64
	withMergeChunkSize(chunk, func() {
		prev := newStore(cloneKV(prevKV), policy, valueType)
		partial := newPartialStore(cloneKV(partialKV), policy, valueType, nil)
		require.NoError(t, prev.Merge(partial))

		snap, err := saveToMap(prev.kvImpl.Save())
		require.NoError(t, err)
		for k, v := range snap {
			out[k] = string(v)
		}
		size = prev.totalSizeBytes
	})
	return out, size
}

// TestMerge_ChunkingInvariance proves that splitting the partial into windows
// produces byte-identical results and identical size accounting versus a single
// pass, across every policy and at exact/off-by-one boundary sizes.
func TestMerge_ChunkingInvariance(t *testing.T) {
	const n = 40

	key := func(i int) string { return fmt.Sprintf("k%04d", i) }

	cases := []struct {
		name      string
		policy    pbsubstreams.Module_KindStore_UpdatePolicy
		valueType string
		prev      map[string][]byte
		partial   map[string][]byte
	}{
		{
			name:      "set_string_cheap_path", // uses GetManySizes, not GetMany
			policy:    pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
			valueType: manifest.OutputValueTypeString,
			prev: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i += 2 { // overwrite half
					m[key(i)] = []byte(fmt.Sprintf("old-%d", i))
				}
				return m
			}(),
			partial: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i++ {
					m[key(i)] = []byte(fmt.Sprintf("new-%d", i))
				}
				return m
			}(),
		},
		{
			name:      "set_if_not_exists_string", // existence spans chunks
			policy:    pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS,
			valueType: manifest.OutputValueTypeString,
			prev: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i += 2 {
					m[key(i)] = []byte(fmt.Sprintf("keep-%d", i))
				}
				return m
			}(),
			partial: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i++ {
					m[key(i)] = []byte(fmt.Sprintf("cand-%d", i))
				}
				return m
			}(),
		},
		{
			name:      "add_int64_fold_path", // uses GetMany, folds existing value
			policy:    pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD,
			valueType: manifest.OutputValueTypeInt64,
			prev: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i += 3 {
					m[key(i)] = []byte(fmt.Sprintf("%d", i))
				}
				return m
			}(),
			partial: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i++ {
					m[key(i)] = []byte(fmt.Sprintf("%d", 100+i))
				}
				return m
			}(),
		},
		{
			name:      "append_string", // concatenation across chunks
			policy:    pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND,
			valueType: manifest.OutputValueTypeString,
			prev: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i += 2 {
					m[key(i)] = []byte("a")
				}
				return m
			}(),
			partial: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i++ {
					m[key(i)] = []byte("b")
				}
				return m
			}(),
		},
		{
			name:      "max_int64",
			policy:    pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX,
			valueType: manifest.OutputValueTypeInt64,
			prev: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i++ {
					m[key(i)] = []byte(fmt.Sprintf("%d", i))
				}
				return m
			}(),
			partial: func() map[string][]byte {
				m := map[string][]byte{}
				for i := 0; i < n; i++ {
					m[key(i)] = []byte(fmt.Sprintf("%d", n-i))
				}
				return m
			}(),
		},
	}

	// Sizes hit every boundary regime: single-key windows, small windows, exact
	// multiples, off-by-one around the key count, and one big-enough-for-one-pass.
	sizes := []int{1, 2, 7, n - 1, n, n + 1, 10_000}

	for _, c := range cases {
		wantKV, wantSize := runMerge(t, c.prev, c.partial, c.policy, c.valueType, 10_000)
		require.Len(t, wantKV, n, "%s: baseline should cover all keys", c.name)

		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/chunk=%d", c.name, size), func(t *testing.T) {
				gotKV, gotSize := runMerge(t, c.prev, c.partial, c.policy, c.valueType, size)
				assert.Equal(t, wantKV, gotKV, "merged KV must not depend on chunk size")
				assert.Equal(t, wantSize, gotSize, "totalSizeBytes accounting must not depend on chunk size")
			})
		}
	}
}

// TestMerge_ChunkingExplicitValues pins concrete merged values for an ADD merge
// whose keys straddle multiple 3-key windows, so a regression in cross-chunk
// folding fails loudly rather than only via the invariance comparison.
func TestMerge_ChunkingExplicitValues(t *testing.T) {
	prev := map[string][]byte{
		"k0": []byte("1"),
		"k2": []byte("2"),
		"k4": []byte("4"),
	}
	partial := map[string][]byte{
		"k0": []byte("10"), // folds with prev -> 11
		"k1": []byte("20"), // new -> 20
		"k2": []byte("30"), // folds -> 32
		"k3": []byte("40"), // new -> 40
		"k4": []byte("50"), // folds -> 54
		"k5": []byte("60"), // new -> 60
		"k6": []byte("70"), // new -> 70
	}

	got, _ := runMerge(t, prev, partial, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, manifest.OutputValueTypeInt64, 3)

	assert.Equal(t, map[string]string{
		"k0": "11",
		"k1": "20",
		"k2": "32",
		"k3": "40",
		"k4": "54",
		"k5": "60",
		"k6": "70",
	}, got)
}

// TestMerge_ChunkingEmptyPartial ensures an empty partial merges cleanly (loop
// body never runs) and leaves the full store untouched.
func TestMerge_ChunkingEmptyPartial(t *testing.T) {
	got, _ := runMerge(t,
		map[string][]byte{"k0": []byte("keep")},
		map[string][]byte{},
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		manifest.OutputValueTypeString,
		3,
	)
	assert.Equal(t, map[string]string{"k0": "keep"}, got)
}
