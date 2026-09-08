package store

import (
	"fmt"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mmapStore builds a mmap-backed FullKV loaded with kv. This mirrors newStore
// but on the mmap backend, which the existing merge tests never exercise
// (newStore/newPartialStore hardcode newMemoryKVImpl) — the untested path that
// backprocessing (partial squash / Merge) actually runs in production.
func mmapFull(t *testing.T, kv map[string][]byte, up pbsubstreams.Module_KindStore_UpdatePolicy, vt string) *FullKV {
	t.Helper()
	impl, err := newMmapKVImplWithConfig("full", "h", &MmapBackendConfig{ScratchSpace: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })
	_, err = impl.Load(mapToIter(kv)); require.NoError(t, err)
	return &FullKV{baseStore: &baseStore{
		kvImpl: impl, kvOps: &pbssinternal.Operations{},
		Config:                  &Config{updatePolicy: up, valueType: vt},
		logger:                  zap.NewNop(),
		recentlyDeletedPrefixes: make(DeletedPrefixes),
	}}
}

func mmapPartial(t *testing.T, kv map[string][]byte, up pbsubstreams.Module_KindStore_UpdatePolicy, vt string) *PartialKV {
	t.Helper()
	impl, err := newMmapKVImplWithConfig("partial", "h", &MmapBackendConfig{ScratchSpace: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })
	_, err = impl.Load(mapToIter(kv)); require.NoError(t, err)
	return &PartialKV{
		baseStore: &baseStore{kvImpl: impl, Config: &Config{updatePolicy: up, valueType: vt}, recentlyDeletedPrefixes: make(map[string]struct{})},
		seen:      make(map[string]bool),
	}
}

// TestMergeMmap_AddBigDecimal_MatchesMemory reproduces the failing scenario:
// an ADD/bigdecimal store (store_derived_factory_tvl's policy) merged from a
// partial, with the uniswap "...Untracked" key shapes, on BOTH backends.
// Memory is the oracle. If mmap's merged bytes differ, the bug is in the mmap
// merge path.
func TestMergeMmap_AddBigDecimal_MatchesMemory(t *testing.T) {
	up := pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD
	vt := manifest.OutputValueTypeBigDecimal

	prevKV := map[string][]byte{}
	partKV := map[string][]byte{}
	suffixes := []string{"totalValueLockedETH", "totalValueLockedETHUntracked", "totalValueLockedUSD", "totalValueLockedUSDUntracked"}
	for i := 0; i < 400; i++ {
		for _, s := range suffixes {
			k := fmt.Sprintf("pool:%040x:%s", i, s)
			prevKV[k] = []byte(fmt.Sprintf("%d.5", i))
			partKV[k] = []byte(fmt.Sprintf("%d.25", i))
		}
	}

	// memory oracle
	memFull := newStore(cloneKV(prevKV), up, vt)
	require.NoError(t, memFull.Merge(newPartialStore(cloneKV(partKV), up, vt, nil)))

	// mmap under test
	mmFull := mmapFull(t, cloneKV(prevKV), up, vt)
	require.NoError(t, mmFull.Merge(mmapPartial(t, cloneKV(partKV), up, vt)))

	require.Equal(t, memFull.kvImpl.KeyCount(), mmFull.kvImpl.KeyCount(), "key count after merge")

	diffs := 0
	require.NoError(t, memFull.kvImpl.Iter(func(k string, memV []byte) error {
		mmV, found := mmFull.kvImpl.Get(k)
		if !found {
			if diffs < 5 {
				t.Logf("MISSING in mmap: %q", k)
			}
			diffs++
			return nil
		}
		if string(mmV) != string(memV) {
			if diffs < 5 {
				t.Logf("DIVERGENCE key %q:\n  mmap   %q\n  memory %q", k, mmV, memV)
			}
			diffs++
		}
		return nil
	}))
	// phantom keys in mmap not in memory
	require.NoError(t, mmFull.kvImpl.Iter(func(k string, v []byte) error {
		if _, found := memFull.kvImpl.Get(k); !found {
			if diffs < 10 {
				t.Logf("PHANTOM in mmap: %q -> %q", k, v)
			}
			diffs++
		}
		return nil
	}))

	require.Zero(t, diffs, "mmap merge diverged from memory oracle in %d entries", diffs)
}

func cloneKV(in map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(in))
	for k, v := range in {
		c := make([]byte, len(v))
		copy(c, v)
		out[k] = c
	}
	return out
}
