package cache

import (
	"context"
	"errors"
	"iter"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/execout"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/stretchr/testify/require"
)

// fakeFileReader implements execout.FileReader with canned Get results.
type fakeFileReader struct {
	payload []byte
	found   bool
	err     error
}

func (f *fakeFileReader) ReadNext() (*pboutput.Item, error) { return nil, nil }
func (f *fakeFileReader) Iter() iter.Seq2[*pboutput.Item, error] {
	return func(yield func(*pboutput.Item, error) bool) {}
}
func (f *fakeFileReader) Get(ctx context.Context, blockNumber uint64) ([]byte, bool, error) {
	return f.payload, f.found, f.err
}
func (f *fakeFileReader) ModuleName() string { return "test_module" }
func (f *fakeFileReader) Filename() string   { return "test-file" }
func (f *fakeFileReader) Close() error       { return nil }

func TestEngineNewBuffer_ExistingExecOutReadErrorIsReturned(t *testing.T) {
	readErr := errors.New("corrupted execout file")
	engine, err := NewEngine(context.Background(), nil, "sf.test.v1.Block", map[string]execout.FileReader{
		"test_module": &fakeFileReader{err: readErr},
	}, nil)
	require.NoError(t, err)

	_, err = engine.NewBuffer(nil, &pbsubstreams.Clock{Id: "block-1", Number: 1}, nil)
	require.ErrorIs(t, err, readErr)
}

func TestEngineNewBuffer_ExistingExecOutAbsentBlockIsSkipped(t *testing.T) {
	engine, err := NewEngine(context.Background(), nil, "sf.test.v1.Block", map[string]execout.FileReader{
		"test_module": &fakeFileReader{found: false},
	}, nil)
	require.NoError(t, err)

	buf, err := engine.NewBuffer(nil, &pbsubstreams.Clock{Id: "block-1", Number: 1}, nil)
	require.NoError(t, err)
	require.NotNil(t, buf)
}

func TestEngineNewBuffer_ExistingExecOutFoundIsSet(t *testing.T) {
	engine, err := NewEngine(context.Background(), nil, "sf.test.v1.Block", map[string]execout.FileReader{
		"test_module": &fakeFileReader{payload: []byte("payload"), found: true},
	}, nil)
	require.NoError(t, err)

	buf, err := engine.NewBuffer(nil, &pbsubstreams.Clock{Id: "block-1", Number: 1}, nil)
	require.NoError(t, err)

	val, cached, err := buf.Get("test_module")
	require.NoError(t, err)
	require.True(t, cached)
	require.Equal(t, []byte("payload"), val)
}
