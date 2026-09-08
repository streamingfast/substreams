package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMmapInitialSizeForLimit(t *testing.T) {
	const floor = defaultInitialMmapSize
	const maxReservation = 8 << 30

	cases := []struct {
		name  string
		limit uint64
		want  int
	}{
		{"zero falls to floor", 0, floor},
		{"tiny limit clamps to floor", 1 << 20, floor},
		{"one gib reserves 1.5x", 1 << 30, int(1<<30 + (1<<30)/2)},
		{"huge limit clamps to cap", 100 << 30, maxReservation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, mmapInitialSizeForLimit(c.limit))
		})
	}
}

// TestMmapPreSizedStoreGrowsBeyondReservation guards the correctness side of the
// InitialMmapSize optimisation: pre-reserving mmap space is a perf hint only, so
// a store opened with a deliberately tiny reservation must still grow past it
// (forcing bbolt to remap) without losing or corrupting any data.
func TestMmapPreSizedStoreGrowsBeyondReservation(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("grow", "h", &MmapBackendConfig{
		ScratchSpace:    t.TempDir(),
		InitialMmapSize: 64 << 10, // 64 KiB — far below the data we load
	})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })

	kv := map[string][]byte{}
	for i := 0; i < 5000; i++ {
		kv[fmt.Sprintf("key:%06d", i)] = []byte(fmt.Sprintf("value-payload-%d-padding-padding-padding", i))
	}

	_, err = impl.Load(mapToIter(kv))
	require.NoError(t, err)
	require.Equal(t, len(kv), impl.KeyCount())

	for k, want := range kv {
		got, found := impl.Get(k)
		require.True(t, found, "key %q missing after growth", k)
		require.Equal(t, want, got, "value corrupted for key %q", k)
	}
}
