package store

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestSetMetadataDetached_SurvivesAndUsesLiveCtx pins the tier1/tier2 metadata
// fix (2ebbfa21 + its tier1 twin): the fire-and-forget metadata write must NOT
// ride the caller's request context. If it did, a request that finished or was
// canceled the instant after triggering the write would kill it. This test
// hands SetMetadataDetached a store whose SetMetadata blocks until it observes
// its own ctx, then confirms the write still runs with a LIVE (non-canceled)
// context even though nothing keeps a request ctx alive.
func TestSetMetadataDetached_SurvivesAndUsesLiveCtx(t *testing.T) {
	type result struct {
		called   bool
		ctxErr   error
		filename string
		meta     map[string]string
	}
	done := make(chan result, 1)

	mock := dstore.NewMockStore(nil)
	mock.SetMetadataFunc = func(ctx context.Context, base string, metadata map[string]string) error {
		done <- result{called: true, ctxErr: ctx.Err(), filename: base, meta: metadata}
		return nil
	}

	SetMetadataDetached(mock, "0000-0100.kv", "store_x", map[string]string{"datasize": "42"}, zap.NewNop())

	select {
	case r := <-done:
		require.True(t, r.called, "SetMetadata must be invoked")
		require.NoError(t, r.ctxErr, "detached write must run on a live context, not a canceled request ctx")
		require.Equal(t, "0000-0100.kv", r.filename)
		require.Equal(t, "42", r.meta["datasize"])
	case <-time.After(5 * time.Second):
		t.Fatal("SetMetadataDetached never invoked SetMetadata")
	}
}

// TestSetMetadataDetached_DoesNotBlockCaller ensures the helper returns
// immediately (it spawns the write in a goroutine) even if the underlying
// SetMetadata is slow — the caller (request teardown) must never block on it.
func TestSetMetadataDetached_DoesNotBlockCaller(t *testing.T) {
	release := make(chan struct{})
	mock := dstore.NewMockStore(nil)
	mock.SetMetadataFunc = func(ctx context.Context, base string, metadata map[string]string) error {
		<-release // block until the test lets it finish
		return nil
	}

	start := time.Now()
	SetMetadataDetached(mock, "f.kv", "s", map[string]string{}, zap.NewNop())
	require.Less(t, time.Since(start), time.Second, "helper must not block the caller on the metadata write")

	close(release) // let the background goroutine finish cleanly
}

// TestSetDataSizeDetached checks that a full KV file's size is written to its metadata only
// when the metadata read before the load does not hold the size it loaded with, without
// changing the caller's map.
func TestSetDataSizeDetached(t *testing.T) {
	tests := []struct {
		name          string
		filename      string
		metadata      map[string]string
		expectedWrite map[string]string
	}{
		{"no file", "", nil, nil},
		{"no metadata", "0000-0100.kv", nil, map[string]string{"datasize": "42"}},
		{"metadata without the size", "0000-0100.kv", map[string]string{"other": "value"}, map[string]string{"other": "value", "datasize": "42"}},
		{"metadata with an empty size", "0000-0100.kv", map[string]string{"datasize": ""}, map[string]string{"datasize": "42"}},
		{"metadata holding another size", "0000-0100.kv", map[string]string{"datasize": "40", "other": "value"}, map[string]string{"datasize": "42", "other": "value"}},
		{"metadata holding the size", "0000-0100.kv", map[string]string{"datasize": "42"}, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			written := make(chan map[string]string, 1)
			mock := dstore.NewMockStore(nil)
			mock.SetMetadataFunc = func(_ context.Context, _ string, metadata map[string]string) error {
				written <- metadata
				return nil
			}
			original := maps.Clone(test.metadata)

			SetDataSizeDetached(mock, test.filename, "store_x", test.metadata, 42, zap.NewNop())

			select {
			case metadata := <-written:
				require.Equal(t, test.expectedWrite, metadata)
			case <-time.After(200 * time.Millisecond):
				require.Nil(t, test.expectedWrite, "SetMetadata was never called")
			}
			require.Equal(t, original, test.metadata, "the caller's metadata must not be changed")
		})
	}
}
