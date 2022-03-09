package state

import (
	"bytes"
	"context"
	"fmt"
	"github.com/streamingfast/dstore"
	"io/ioutil"
)

type IOFactory interface {
	New(name string) StateIO
}

type StoreStateIOFactory struct {
	store dstore.Store
}

func NewStoreStateIOFactory(store dstore.Store) IOFactory {
	return &StoreStateIOFactory{store: store}
}

func (f *StoreStateIOFactory) New(name string) StateIO {
	return &StoreStateIO{
		name:  name,
		store: f.store,
	}
}

type StateIO interface {
	WriteState(ctx context.Context, content []byte, blockNum uint64) error
	ReadState(ctx context.Context, blockNum uint64) ([]byte, error)

	WritePartial(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error
	ReadPartial(ctx context.Context, startBlockNum, endBlockNum uint64) ([]byte, error)
}

type StoreStateIO struct {
	name  string
	store dstore.Store
}

func (s *StoreStateIO) WriteState(ctx context.Context, content []byte, blockNum uint64) error {
	return s.store.WriteObject(ctx, GetStateFileName(s.name, blockNum), bytes.NewReader(content))
}

func (s *StoreStateIO) ReadState(ctx context.Context, blockNum uint64) ([]byte, error) {
	objectName := GetStateFileName(s.name, blockNum)
	obj, err := s.store.OpenObject(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", objectName, err)
	}

	data, err := ioutil.ReadAll(obj)
	return data, err
}

func (s *StoreStateIO) WritePartial(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error {
	return s.store.WriteObject(ctx, GetPartialFileName(s.name, startBlockNum, endBlockNum), bytes.NewReader(content))
}

func (s *StoreStateIO) ReadPartial(ctx context.Context, startBlockNum, endBlockNum uint64) ([]byte, error) {
	objectName := GetPartialFileName(s.name, startBlockNum, endBlockNum)
	obj, err := s.store.OpenObject(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", objectName, err)
	}

	data, err := ioutil.ReadAll(obj)
	return data, err
}

func GetStateFileName(name string, blockNum uint64) string {
	return fmt.Sprintf("%d-%s.kv", blockNum, name)
}

func GetPartialFileName(name string, startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%d-%d-%s.partial", startBlockNum, endBlockNum, name)
}
