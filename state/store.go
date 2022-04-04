package state

import (
	"bytes"
	"context"
	"fmt"

	"github.com/streamingfast/dstore"
)

type FactoryInterface interface {
	New(name string, moduleHash string) Store
}

type StoreFactory struct {
	store dstore.Store
}

func NewStoreFactory(store dstore.Store) *StoreFactory {
	return &StoreFactory{
		store: store,
	}
}

func (f *StoreFactory) New(name string, moduleHash string) Store {
	return NewStore(name, moduleHash, f.store)
}

type Store interface {
	dstore.Store

	WriteState(ctx context.Context, content []byte, blockNum uint64) error
	WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error
}

type DefaultStore struct {
	dstore.Store

	name       string
	moduleHash string
}

func NewStore(name string, moduleHash string, baseStore dstore.Store) *DefaultStore {
	s := &DefaultStore{
		Store:      baseStore,
		name:       name,
		moduleHash: moduleHash,
	}

	return s
}

func (s *DefaultStore) WriteState(ctx context.Context, content []byte, blockNum uint64) error {
	return s.WriteObject(ctx, s.stateFileName(blockNum), bytes.NewReader(content))
}

func (s *DefaultStore) WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error {
	return s.WriteObject(ctx, s.partialFileName(startBlockNum, endBlockNum), bytes.NewReader(content))
}

func (s *DefaultStore) stateFileName(blockNum uint64) string {
	return fmt.Sprintf("%s-%s-%d.kv", s.moduleHash, s.name, blockNum)
}

func (s *DefaultStore) partialFileName(startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%s-%d-%d.partial", s.moduleHash, s.name, endBlockNum, startBlockNum)
}
