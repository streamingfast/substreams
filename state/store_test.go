package state

import (
	"context"

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

type TestFactory struct {
	stores map[string]*TestStore
}

func (t *TestFactory) New(name string, moduleHash string) Store {
	if _, ok := t.stores[name]; ok {
		return t.stores[name]
	}
	return nil
}
