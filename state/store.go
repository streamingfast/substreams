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

func NewStore(name string, moduleHash string, moduleStartBlock uint64, baseStore dstore.Store) *Store {
	s := &Store{
		Store:            baseStore,
		Name:             name,
		ModuleHash:       moduleHash,
		ModuleStartBlock: moduleStartBlock,
	}

	return s
}

func (s *Store) WriteState(ctx context.Context, content []byte, blockNum uint64) error {
	return s.WriteObject(ctx, s.StateFileName(blockNum), bytes.NewReader(content))
}

func (s *Store) WritePartialState(ctx context.Context, content []byte, startBlockNum, endBlockNum uint64) error {
	return s.WriteObject(ctx, s.PartialFileName(startBlockNum, endBlockNum), bytes.NewReader(content))
}

func (s *Store) StateFileName(blockNum uint64) string {
	return fmt.Sprintf("%s-%s-%d-%d.kv", s.ModuleHash, s.Name, blockNum, s.ModuleStartBlock)
}

func (s *Store) PartialFileName(startBlockNum, endBlockNum uint64) string {
	return fmt.Sprintf("%s-%s-%d-%d.partial", s.ModuleHash, s.Name, endBlockNum, startBlockNum)
}
