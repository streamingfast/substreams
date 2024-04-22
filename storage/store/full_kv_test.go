package store

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFullKV_Save_Load_Empty_MapNotNil(t *testing.T) {
	var writtenBytes []byte
	store := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	kvs := &FullKV{
		baseStore: &baseStore{
			kv: map[string][]byte{},

			pendingOps: &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	file, writer, err := kvs.Save(123)
	require.NoError(t, err)

	err = writer.Write(context.Background())
	require.NoError(t, err)

	kvl := &FullKV{
		baseStore: &baseStore{
			kv: map[string][]byte{},

			pendingOps: &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	err = kvl.Load(context.Background(), file)
	require.NoError(t, err)
	require.NotNilf(t, kvl.kv, "kvl.kv is nil")
}
