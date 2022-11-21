package store

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPartialKV_Save_Load_Empty_MapNotNil(t *testing.T) {
	var writtenBytes []byte
	store := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	kvs := &PartialKV{
		baseStore: &baseStore{
			kv: map[string][]byte{},

			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	br, writer, err := kvs.Save(123)
	require.NoError(t, err)

	err = writer.Write(context.Background())
	require.NoError(t, err)

	kvl := &PartialKV{
		baseStore: &baseStore{
			kv: map[string][]byte{},

			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	err = kvl.Load(context.Background(), br.ExclusiveEndBlock)
	require.NoError(t, err)
	require.NotNilf(t, kvl.kv, "kvl.kv is nil")
}
