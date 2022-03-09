package state

import (
	"context"
	"github.com/streamingfast/bstream"
)

type TestIO struct {
	WriteStateFunc func(ctx context.Context, content []byte, block *bstream.Block) error
	ReadStateFunc  func(ctx context.Context, blockNum uint64) ([]byte, error)
}

func (io *TestIO) WriteState(ctx context.Context, content []byte, block *bstream.Block) error {
	if io.WriteStateFunc != nil {
		return io.WriteStateFunc(ctx, content, block)
	}
	return nil
}

func (io *TestIO) ReadState(ctx context.Context, blockNum uint64) ([]byte, error) {
	if io.ReadStateFunc != nil {
		return io.ReadStateFunc(ctx, blockNum)
	}
	return nil, nil
}
