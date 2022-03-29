package state

import (
	"bytes"
	"context"
	"fmt"

	"github.com/streamingfast/dstore"
)

type FactoryInterface interface {
	New(name string) StoreInterface
}

type StoreFactory struct {
	store dstore.Store
}

func NewStoreFactory(store dstore.Store) *StoreFactory {
	return &StoreFactory{
		store: store,
	}
}

func (f *StoreFactory) New(name string) StoreInterface {
	return NewStore(name, f.store)
}

type StoreInterface interface {
	dstore.Store

	WriteState(ctx context.Context, content []byte, blockNum uint64) error
	WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error
}

type Store struct {
	dstore.Store

	name string
}

func NewStore(name string, baseStore dstore.Store) *Store {
	s := &Store{
		Store: baseStore,
		name:  name,
	}

	return s
}

func (s *Store) WriteState(ctx context.Context, content []byte, blockNum uint64) error {
	return s.WriteObject(ctx, GetStateFileName(s.name, blockNum), bytes.NewReader(content))
}

func (s *Store) WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error {
	return s.WriteObject(ctx, GetPartialFileName(s.name, startBlockNum, endBlockNum), bytes.NewReader(content))
}

func GetStateFileName(name string, blockNum uint64) string {
	return fmt.Sprintf("%s-%d.kv", name, blockNum)
}

func GetPartialFileName(name string, startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%d-%d.partial", name, endBlockNum, startBlockNum)
}
