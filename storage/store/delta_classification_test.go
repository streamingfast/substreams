package store

import (
	"errors"
	"strings"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// bigConfig lifts totalSizeLimit above the entry sizes under test so the
// store-too-big panic never fires before the check we are exercising.
var bigConfig = &Config{name: "test", totalSizeLimit: 1 << 40}

func recoverApplyDelta(s *baseStore, delta *pbsubstreams.StoreDelta) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
				return
			}
			err = errors.New(r.(string))
		}
	}()
	s.ApplyDelta(delta)
	return nil
}

// An oversized key must be rejected identically on both backends (deterministic,
// ErrStoreEntryTooLarge) — otherwise the module would succeed on memory and fail
// only on mmap.
func TestApplyDelta_OversizedKey_DeterministicOnBothBackends(t *testing.T) {
	bigKey := strings.Repeat("k", maxStoreKeySize+1)

	backends := map[string]func(t *testing.T) KVImpl{
		"memory": func(t *testing.T) KVImpl { return newMemoryKVImpl() },
		"mmap": func(t *testing.T) KVImpl {
			impl, err := newMmapKVImplWithConfig("test", "hash", &MmapBackendConfig{ScratchSpace: t.TempDir()})
			require.NoError(t, err)
			return impl
		},
	}

	for name, build := range backends {
		t.Run(name, func(t *testing.T) {
			kvImpl := build(t)
			defer kvImpl.Close()

			s := &baseStore{Config: bigConfig, kvImpl: kvImpl, logger: zap.NewNop()}
			err := recoverApplyDelta(s, &pbsubstreams.StoreDelta{
				Operation: pbsubstreams.StoreDelta_CREATE,
				Key:       bigKey,
				NewValue:  []byte("v"),
			})

			require.Error(t, err)
			require.ErrorIs(t, err, ErrStoreEntryTooLarge, "oversized key must be a deterministic entry-too-large error")
			require.NotErrorIs(t, err, ErrStoreBackendFailure)
		})
	}
}

// A backend/infra failure (here: a closed mmap db) must surface as
// ErrStoreBackendFailure — non-deterministic — and NOT as an entry-too-large or
// generic deterministic error, so callers retry instead of caching it.
func TestApplyDelta_BackendFailure_NonDeterministic(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("test", "hash", &MmapBackendConfig{ScratchSpace: t.TempDir()})
	require.NoError(t, err)
	require.NoError(t, impl.Close()) // now every Set/Delete fails with "database not open"

	s := &baseStore{Config: bigConfig, kvImpl: impl}
	applyErr := recoverApplyDelta(s, &pbsubstreams.StoreDelta{
		Operation: pbsubstreams.StoreDelta_CREATE,
		Key:       "k",
		NewValue:  []byte("v"),
	})

	require.Error(t, applyErr)
	require.ErrorIs(t, applyErr, ErrStoreBackendFailure)
	require.NotErrorIs(t, applyErr, ErrStoreEntryTooLarge)
}
