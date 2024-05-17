package execout

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/test-go/testify/require"
	"go.uber.org/zap"
)

func testStore() dstore.Store {
	s := dstore.NewMockStore(nil)
	s.Files = make(map[string][]byte)
	p := pboutput.Map{Kv: make(map[string]*pboutput.Item)}
	data, _ := p.MarshalFast()

	s.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
		cnt, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		s.Files[base] = cnt
		return nil
	}
	s.FileExistsFunc = func(ctx context.Context, base string) (bool, error) {
		return true, nil
	}
	s.OpenObjectFunc = func(ctx context.Context, name string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	return s
}

func NewTestFileWalker() *FileWalker {
	store := testStore()
	config, _ := NewConfig("test", 0, pbsubstreams.ModuleKindMap, "abc", store, zap.NewNop())
	segmenter := block.NewSegmenter(10, 0, 100)
	return NewFileWalker(config, segmenter, zap.NewNop())
}

func TestName(t *testing.T) {
	w := NewTestFileWalker()
	ctx := context.Background()

	w.File()
	require.Len(t, w.buffer, 0)
	w.PreloadNext(ctx)
	require.Len(t, w.buffer, 1)

	w.Next()
	require.Len(t, w.buffer, 1)

	w.File()
	require.Len(t, w.buffer, 0)

	w.PreloadNext(ctx)
	require.Len(t, w.buffer, 1)
}
