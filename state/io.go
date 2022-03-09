package state

import (
	"bytes"
	"context"
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"io/ioutil"
)

type IOFactory interface {
	New(name string) StateIO
}

//type DiskStateIOFactory struct {
//	dataFolder string
//}
//
//func NewDiskStateIOFactory(folder string) IOFactory {
//	return &DiskStateIOFactory{dataFolder: folder}
//}
//
//func (f *DiskStateIOFactory) New(name string) StateIO {
//	return &DiskStateIO{
//		name:       name,
//		dataFolder: f.dataFolder,
//	}
//}

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
	WriteState(ctx context.Context, content []byte, block *bstream.Block) error
	ReadState(ctx context.Context, blockNum uint64) ([]byte, error)
}

type StoreStateIO struct {
	name  string
	store dstore.Store
}

func (s *StoreStateIO) WriteState(ctx context.Context, content []byte, block *bstream.Block) error {
	return s.store.WriteObject(ctx, GetStateFileName(s.name, block), bytes.NewBuffer(content))
}

func (s *StoreStateIO) ReadState(ctx context.Context, blockNum uint64) ([]byte, error) {
	relativeStartBlock := (blockNum / 100) * 100
	block := &bstream.Block{Number: relativeStartBlock}

	objectName := GetStateFileName(s.name, block)
	obj, err := s.store.OpenObject(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", objectName, err)
	}

	data, err := ioutil.ReadAll(obj)
	return data, err
}

func GetStateFileName(name string, block *bstream.Block) string {
	blockNum := (block.Num() / 100) * 100
	return fmt.Sprintf("%d-%s.kv", blockNum, name)
}
