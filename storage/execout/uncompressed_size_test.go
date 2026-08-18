package execout

import (
	"context"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestUncompressedSize_FromMetadata(t *testing.T) {
	store := dstore.NewMockStore(nil)
	// unreadable content: proves the file itself is never opened when the metadata is there
	store.SetFile("0000001000-0000002000.output", []byte("not a valid output file"))
	store.Metadata["0000001000-0000002000.output"] = map[string]string{MetadataDataSize: "4321"}

	size, fromMetadata, err := UncompressedSize(context.Background(), store, zap.NewNop(), block.NewRange(1000, 2000), "mod")
	require.NoError(t, err)
	assert.True(t, fromMetadata)
	assert.Equal(t, uint64(4321), size)
}

func TestUncompressedSize_FromFile(t *testing.T) {
	ctx := context.Background()
	store := dstore.NewMockStore(nil)
	store.SetMetadataFunc = func(context.Context, string, map[string]string) error { return nil } // no metadata backend

	config, err := NewConfig("mod", 0, pbsubstreams.ModuleKindMap, "hash", "hash", store, zap.NewNop())
	require.NoError(t, err)

	writer := config.NewFileWriter(ctx, block.NewRange(1000, 2000))
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1000, Id: "1000"}, []byte("0123456789")))
	require.NoError(t, writer.SetItem(&pbsubstreams.Clock{Number: 1001, Id: "1001"}, []byte("012345")))
	require.NoError(t, writer.Close())

	size, fromMetadata, err := config.UncompressedSize(ctx, block.NewRange(1000, 2000))
	require.NoError(t, err)
	assert.False(t, fromMetadata)
	assert.Equal(t, uint64(16), size)
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

	assert.Equal(t, map[string]string{MetadataDataSize: "10"}, <-written)
}
