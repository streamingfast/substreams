package store

import (
	"context"
	"strconv"
	"testing"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestFullKVSave_WritesDataSize checks, on both backends, that a full KV file is saved with its
// size in its metadata, and that it is the size the file loads with, so that loading it needs
// no metadata write. The store is built from sets, overwrites, a deleted prefix and a merged
// partial, the paths that keep the size up to date.
func TestFullKVSave_WritesDataSize(t *testing.T) {
	for _, backend := range []string{"memory", "mmap"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			objStore := dstore.NewMockStore(nil)
			config, err := NewConfig("test", 0, "test.module.hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", objStore, nil, 0, t.TempDir(), backend)
			require.NoError(t, err)

			saved := config.NewFullKV(zap.NewNop())
			defer saved.Close()
			saved.Set(0, "key1", "value1")
			saved.Set(0, "key2", "a longer value2")
			saved.Set(0, "key1", "replaced")
			saved.DeletePrefix(0, "key2")
			saved.Set(0, "key3", "value3")
			require.NoError(t, saved.Flush())

			partial := config.NewPartialKV(1000, zap.NewNop())
			partial.Set(1000, "key3", "value3 from the partial")
			partial.Set(1000, "key4", "value4")
			require.NoError(t, partial.Flush())
			require.NoError(t, saved.Merge(partial))

			file, writer, err := saved.Save(2000)
			require.NoError(t, err)
			require.NoError(t, writer.Write(ctx))

			attrs, err := saved.Store().ObjectAttributes(ctx, file.Filename)
			require.NoError(t, err)

			loaded := config.NewFullKV(zap.NewNop())
			defer loaded.Close()
			require.NoError(t, loaded.Load(ctx, file))
			require.NotZero(t, loaded.SizeBytes())
			require.Equal(t, strconv.FormatUint(loaded.SizeBytes(), 10), attrs.Metadata["datasize"])
		})
	}
}
