package store

import (
	"fmt"
	"testing"

	"github.com/streamingfast/substreams/storage/store/marshaller"
	"github.com/stretchr/testify/require"
)

// TestMmapSnapshotLoadRoundTrip_ByteExact follows the data from the uniswap
// panic ('Invalid store BigDecimal string DUntracked0.000...'). It reproduces
// the production mmap save/load path — Snapshot -> MarshalStreamSnapshot ->
// UnmarshalIter -> Load — with the exact store shape that panicked
// (store_derived_tvl: set/bigdecimal, keys ending in "...Untracked", decimal
// values), then reads every key back and asserts it is byte-identical to what
// went in. Any corruption localizes the bug to the mmap round-trip.
func TestMmapSnapshotLoadRoundTrip_ByteExact(t *testing.T) {
	src := newTestMmap(t, "src")

	// The failing store's real shape: pool-keyed bigdecimal TVL, incl. the
	// "...Untracked" suffix keys from the panic string.
	want := map[string][]byte{}
	suffixes := []string{
		"totalValueLockedETH",
		"totalValueLockedUSD",
		"totalValueLockedETHUntracked",
		"totalValueLockedUSDUntracked",
	}
	for i := 0; i < 300; i++ {
		for _, s := range suffixes {
			k := fmt.Sprintf("pool:%040x:%s", i, s)
			v := []byte(fmt.Sprintf("%d.0000000000000000000000000000000000000000000000000000000000000%03d", i, i))
			want[k] = v
		}
	}
	require.NoError(t, src.BatchSet(want))

	// Production save path: Snapshot -> stream marshal.
	snap, err := src.Snapshot()
	require.NoError(t, err)
	vt := &marshaller.VTproto{}
	reader := vt.MarshalStreamSnapshot(snap, nil)
	defer reader.Close()

	// Production mmap load path: UnmarshalIter -> Load into a fresh mmap store.
	dst := newTestMmap(t, "dst")
	_, err = dst.Load(vt.UnmarshalIter(reader, 0))
	require.NoError(t, err)

	require.Equal(t, len(want), dst.KeyCount(), "key count after round-trip")

	// Byte-exact verification of every key.
	corrupt := 0
	for k, wantV := range want {
		gotV, found := dst.Get(k)
		if !found {
			if corrupt < 5 {
				t.Logf("MISSING key: %q", k)
			}
			corrupt++
			continue
		}
		if string(gotV) != string(wantV) {
			if corrupt < 5 {
				t.Logf("CORRUPT value for key %q:\n  got  %q\n  want %q", k, gotV, wantV)
			}
			corrupt++
		}
	}

	// Also scan for keys that exist in dst but not in src (phantom/corrupted keys).
	require.NoError(t, dst.Iter(func(k string, v []byte) error {
		if _, ok := want[k]; !ok {
			if corrupt < 10 {
				t.Logf("PHANTOM key in dst: %q -> %q", k, v)
			}
			corrupt++
		}
		return nil
	}))

	require.Zero(t, corrupt, "mmap snapshot/load round-trip corrupted %d entries", corrupt)
}

func newTestMmap(t *testing.T, name string) *mmapKVImpl {
	t.Helper()
	cfg := &MmapBackendConfig{ScratchSpace: t.TempDir()}
	impl, err := newMmapKVImplWithConfig(name, "test_hash", cfg)
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })
	return impl
}
