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

// TestDeletePrefixMmap_OldValueNotAliased reproduces the uniswap panic
// ('Invalid store BigDecimal string DUntracked0.000...') across every store
// value type.
//
// DeletePrefix's Scan callback stored the bbolt-aliased `val` directly into
// delta.OldValue and appended the delta to a slice that OUTLIVES the Scan
// transaction. On the mmap backend, bbolt values are only valid during the
// View transaction — once it closes, that memory is reused, so OldValue points
// at adjacent page bytes (e.g. a neighbouring "...Untracked" key). A downstream
// module reading the delete-delta's OldValue as its declared type (BigDecimal,
// string, bigint, ...) then sees corruption.
//
// The corruption is value-type agnostic (it is raw byte aliasing), but a
// downstream consumer parses OldValue according to the store's valueType, so we
// exercise each: any type whose parser rejects the garbage bytes would panic in
// production the way BigDecimal did. This test fails (red) on the aliased code
// and passes once OldValue is copied.
func TestDeletePrefixMmap_OldValueNotAliased(t *testing.T) {
	// value producers per declared store type — realistic bytes a real module
	// would store and later re-parse.
	types := []struct {
		name     string
		valType  string
		valueFor func(i int) []byte
	}{
		{"bigdecimal", manifest.OutputValueTypeBigDecimal, func(i int) []byte {
			return []byte(fmt.Sprintf("%d.000000000000000000000000000000000000000000000000000%03d", i, i))
		}},
		{"string", manifest.OutputValueTypeString, func(i int) []byte {
			return []byte(fmt.Sprintf("value-string-payload-%08d", i))
		}},
		{"bigint", manifest.OutputValueTypeBigInt, func(i int) []byte {
			return []byte(fmt.Sprintf("%d000000000000000000000000000%d", i, i))
		}},
		{"int64", manifest.OutputValueTypeInt64, func(i int) []byte {
			return []byte(fmt.Sprintf("%d", int64(i)*1_000_003))
		}},
		{"float64", manifest.OutputValueTypeFloat64, func(i int) []byte {
			return []byte(fmt.Sprintf("%d.5", i))
		}},
		{"bytes-proto", "bytes", func(i int) []byte {
			// arbitrary binary-ish payload (proto stores hold raw bytes)
			b := make([]byte, 24)
			for j := range b {
				b[j] = byte((i + j) % 251)
			}
			return b
		}},
	}

	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			impl, err := newMmapKVImplWithConfig("del_"+tc.name, "h", &MmapBackendConfig{ScratchSpace: t.TempDir()})
			require.NoError(t, err)
			t.Cleanup(func() { impl.Close() })

			// Keys under the deleted prefix, interleaved with adjacent
			// "...Untracked" keys — the neighbours whose bytes bleed into an
			// aliased value in bbolt's sorted b-tree.
			want := map[string][]byte{}
			for i := 0; i < 500; i++ {
				want[fmt.Sprintf("PoolDayData:%08d:tvl", i)] = tc.valueFor(i)
				want[fmt.Sprintf("PoolDayData:%08d:totalValueLockedUSDUntracked", i)] = tc.valueFor(i + 100000)
			}
			require.NoError(t, impl.BatchSet(want))

			b := &baseStore{
				kvImpl:                  impl,
				kvOps:                   &pbssinternal.Operations{},
				Config:                  &Config{updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, valueType: tc.valType},
				logger:                  zap.NewNop(),
				recentlyDeletedPrefixes: make(map[string]struct{}),
			}

			// Capture DELETE deltas (each carrying OldValue) for the prefix.
			b.deletePrefix(0, "PoolDayData:")

			// Force bbolt page reuse after the scan: subsequent writes reclaim
			// the pages an aliased OldValue would point at.
			for i := 0; i < 1000; i++ {
				_ = impl.Set(fmt.Sprintf("churn:%08d", i), []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
			}

			corrupt := 0
			for _, d := range b.GetDeltas() {
				if d.Operation != pbsubstreams.StoreDelta_DELETE {
					continue
				}
				wantV, ok := want[d.Key]
				if !ok {
					continue
				}
				if string(d.OldValue) != string(wantV) {
					if corrupt < 5 {
						t.Logf("ALIASED OldValue (%s) key %q:\n  got  %q\n  want %q", tc.name, d.Key, d.OldValue, wantV)
					}
					corrupt++
				}
			}
			require.Zero(t, corrupt, "DeletePrefix captured %d aliased OldValues for type %s", corrupt, tc.name)
		})
	}
}
