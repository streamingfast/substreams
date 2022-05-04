package state

import (
	"bytes"
	"context"
	"fmt"

	"github.com/streamingfast/dstore"
)

type Store struct {
	dstore.Store

	Name             string
	ModuleHash       string
	ModuleStartBlock uint64
}

func newStoreWithStore(name string, moduleHash string, moduleStartBlock uint64, store dstore.Store) (*Store, error) {
	s := &Store{
		Store:            store,
		Name:             name,
		ModuleHash:       moduleHash,
		ModuleStartBlock: moduleStartBlock,
	}

	return s, nil
}

func NewStore(name string, moduleHash string, moduleStartBlock uint64, baseStore dstore.Store) (*Store, error) {
	ds, err := baseStore.SubStore(fmt.Sprintf("%s-%s", name, moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating new store: %w", err)
	}
	s := &Store{
		Store:            ds,
		Name:             name,
		ModuleHash:       moduleHash,
		ModuleStartBlock: moduleStartBlock,
	}

	return s, nil
}

func (s *Store) WriteState(ctx context.Context, content []byte, blockNum uint64) (string, error) {
	filename := s.StateFileName(blockNum)
	return filename, s.WriteObject(ctx, s.StateFileName(blockNum), bytes.NewReader(content))
}

func (s *Store) WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) (string, error) {
	filename := s.PartialFileName(startBlockNum, endBlockNum)
	return filename, s.WriteObject(ctx, s.PartialFileName(startBlockNum, endBlockNum), bytes.NewReader(content))
}

func (s *Store) StateFilePrefix(blockNum uint64) string {
	return fmt.Sprintf("%s-%010d", s.Name, blockNum)
}

func (s *Store) StateFileName(blockNum uint64) string {
	return fmt.Sprintf("%s-%010d-%010d.kv", s.Name, blockNum, s.ModuleStartBlock)
}

func (s *Store) PartialFileName(startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%010d-%010d.partial", s.Name, endBlockNum, startBlockNum)
}

func StateFilePrefix(storeName string, blockNum uint64) string {
	return fmt.Sprintf("%s-%010d", storeName, blockNum)
}

func PartialFileName(storeName string, startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%010d-%010d.partial", storeName, endBlockNum, startBlockNum)
}

func StateFileName(storeName string, startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%010d-%010d.kv", storeName, endBlockNum, startBlockNum)
}

func FilePrefix(storeName string, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%010d-", storeName, endBlockNum)
}
