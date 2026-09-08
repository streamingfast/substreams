package execout

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOutputStats_FromMetadata(t *testing.T) {
	store := dstore.NewMockStore(nil)
	// unreadable content: proves the file itself is never opened when the metadata is there
	store.SetFile("0000001000-0000002000.output", []byte("not a valid output file"))
	store.Metadata["0000001000-0000002000.output"] = map[string]string{MetadataDataSize: "4321", MetadataItemCount: "17"}

	size, items, fromMetadata, err := OutputStats(context.Background(), store, zap.NewNop(), block.NewRange(1000, 2000), "mod")
	require.NoError(t, err)
	assert.True(t, fromMetadata)
	assert.Equal(t, uint64(4321), size)
	assert.Equal(t, uint64(17), items)
}

// TestOutputStats_PartialMetadata: the two keys are written together, so a file carrying only
// one of them was written before the other existed and has to be read for both.
func TestOutputStats_PartialMetadata(t *testing.T) {
	ctx := context.Background()
	store := dstore.NewMockStore(nil)
	store.SetMetadataFunc = func(context.Context, string, map[string]string) error { return nil }

	config, err := NewConfig("mod", 0, pbsubstreams.ModuleKindMap, "hash", "hash", store, zap.NewNop())
	require.NoError(t, err)

	writer := config.NewFileWriter(ctx, block.NewRange(1000, 2000))
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1000, Id: "1000"}, []byte("0123456789")))
	require.NoError(t, writer.Close())

	store.Metadata["0000001000-0000002000.output"] = map[string]string{MetadataDataSize: "10"}

	size, items, fromMetadata, err := config.OutputStats(ctx, block.NewRange(1000, 2000))
	require.NoError(t, err)
	assert.False(t, fromMetadata)
	assert.Equal(t, uint64(10), size)
	assert.Equal(t, uint64(1), items)
}

func TestOutputStats_FromFile(t *testing.T) {
	ctx := context.Background()
	store := dstore.NewMockStore(nil)
	store.SetMetadataFunc = func(context.Context, string, map[string]string) error { return nil } // no metadata backend

	config, err := NewConfig("mod", 0, pbsubstreams.ModuleKindMap, "hash", "hash", store, zap.NewNop())
	require.NoError(t, err)

	writer := config.NewFileWriter(ctx, block.NewRange(1000, 2000))
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1000, Id: "1000"}, []byte("0123456789")))
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1001, Id: "1001"}, []byte("012345")))
	require.NoError(t, writer.Close())

	size, items, fromMetadata, err := config.OutputStats(ctx, block.NewRange(1000, 2000))
	require.NoError(t, err)
	assert.False(t, fromMetadata)
	assert.Equal(t, uint64(16), size)
	assert.Equal(t, uint64(2), items)
}

func TestFileWriter_RecordsDataSizeMetadata(t *testing.T) {
	ctx := context.Background()
	store := dstore.NewMockStore(nil)

	written := make(chan map[string]string, 1)
	store.SetMetadataFunc = func(_ context.Context, _ string, metadata map[string]string) error {
		written <- metadata
		return nil
	}

	config, err := NewConfig("mod", 0, pbsubstreams.ModuleKindMap, "hash", "hash", store, zap.NewNop())
	require.NoError(t, err)

	writer := config.NewFileWriter(ctx, block.NewRange(1000, 2000))
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1000, Id: "1000"}, []byte("0123456789")))
	require.NoError(t, writer.Close())

	assert.Equal(t, map[string]string{MetadataDataSize: "10", MetadataItemCount: "1"}, <-written)
}

// s3SchemeStore is a store whose SetMetadata would be a full object rewrite.
type s3SchemeStore struct {
	*dstore.MockStore
}

func (s *s3SchemeStore) BaseURL() *url.URL { return &url.URL{Scheme: "s3", Path: "/bucket"} }

func TestFileWriter_SkipsDataSizeMetadataOnRewriteBackend(t *testing.T) {
	ctx := context.Background()

	called := make(chan map[string]string, 1)
	mock := dstore.NewMockStore(nil)
	mock.SetMetadataFunc = func(_ context.Context, _ string, metadata map[string]string) error {
		called <- metadata
		return nil
	}
	store := &s3SchemeStore{mock}

	writer := NewFileWriter(ctx, store, zap.NewNop(), block.NewRange(1000, 2000), "mod")
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1000, Id: "1000"}, []byte("0123456789")))
	require.NoError(t, writer.Close())

	select {
	case metadata := <-called:
		t.Fatalf("SetMetadata was called on a rewrite backend with %v", metadata)
	case <-time.After(100 * time.Millisecond):
	}
}
