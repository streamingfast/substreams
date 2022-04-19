package state

import (
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
)

type TestStore struct {
	*dstore.MockStore

	WriteStateFunc        func(ctx context.Context, content []byte, blockNum uint64) error
	WritePartialStateFunc func(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error
}

func (io *TestStore) WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error {
	if io.WritePartialStateFunc != nil {
		return io.WritePartialStateFunc(ctx, content, startBlockNum, endBlockNum)
	}
	return nil
}

func (io *TestStore) WriteState(ctx context.Context, content []byte, blockNum uint64) error {
	if io.WriteStateFunc != nil {
		return io.WriteStateFunc(ctx, content, blockNum)
	}
	return nil
}

func TestStateFileName(t *testing.T) {
	s, err := NewStore(
		"test",
		"abc",
		100,
		dstore.NewMockStore(func(base string, f io.Reader) (err error) {
			return nil
		}),
	)

	require.NoError(t, err)

	prefix := s.StateFilePrefix(10000)
	require.Equal(t, "test-0000010000", prefix)

	stateFileName := s.StateFileName(10000)
	require.Equal(t, "test-0000010000-0000000100.kv", stateFileName)

	partialFileName := s.PartialFileName(10000, 20000)
	require.Equal(t, "test-0000020000-0000010000.partial", partialFileName)
}
