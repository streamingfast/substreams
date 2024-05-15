package execout

import (
	"bytes"
	"context"
	"io"
	"testing"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

func testStore() dstore.Store {
	s := dstore.NewMockStore(nil)
	s.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
		cnt, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		s.Files[base] = cnt
		return nil
	}
	s.FileExistsFunc = func(ctx context.Context, base string) (bool, error) {
		_, ok := s.Files[base]
		return ok, nil
	}
	s.OpenObjectFunc = func(ctx context.Context, name string) (io.ReadCloser, error) {
		cnt, ok := s.Files[name]
		if !ok {
			return nil, dstore.ErrNotFound
		}
		return io.NopCloser(bytes.NewReader(cnt)), nil
	}
	s.Files = make(map[string][]byte)
	p := pboutput.Map{Kv: make(map[string]*pboutput.Item)}
	data, _ := p.MarshalFast()

	s.Files["0000000010-0000000020.output"] = data
	s.Files["0000000020-0000000030.output"] = data
	s.Files["0000000030-0000000040.output"] = data
	s.Files["0000000040-0000000050.output"] = data
	s.Files["0000000050-0000000060.output"] = data
	s.Files["0000000060-0000000070.output"] = data

	return s
}

func NewTestFileWalker() *FileWalker {
	store := testStore()
	config, _ := NewConfig("test", 0, pbsubstreams.ModuleKindMap, "abc", store, zap.NewNop())
	segmenter := block.NewSegmenter(10, 0, 100)

	return &FileWalker{
		config:                      config,
		segmenter:                   segmenter,
		segment:                     segmenter.FirstIndex(),
		buffer:                      make(map[int]*File),
		currentlyPreloadingSegments: make(map[int]chan bool),
	}
}

func TestFileWalker_Preloading(t *testing.T) {
	walker := NewTestFileWalker()

	// first preload will only load first segment
	walker.PreloadNext(context.Background())
	assert.Len(t, walker.buffer, 1)

	walker.Next()
	assert.Len(t, walker.buffer, 1)

	// since we are no longer on the first segment, next preload will load the next two segments
	walker.PreloadNext(context.Background())
	assert.Len(t, walker.buffer, 3)
	assert.Len(t, walker.currentlyPreloadingSegments, 3)

	// skip two
	walker.Next()
	walker.Next()

	// two files should have been deleted from the buffer
	assert.Len(t, walker.buffer, 1)
	assert.Len(t, walker.currentlyPreloadingSegments, 1)

	// check that resulting file is preloaded
	f := walker.File()
	assert.True(t, f.preloaded)

	// currently preloading segment should have been removed during the reading of the file
	assert.Len(t, walker.currentlyPreloadingSegments, 0)

	// skip and check that nothing is preloaded anymore
	walker.Next()
	assert.Len(t, walker.buffer, 0)

	// check that resulting file is not preloaded
	f = walker.File()
	assert.False(t, f.preloaded)
}
