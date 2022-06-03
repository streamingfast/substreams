package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/state"
)

type StorageState struct {
	lastBlocks map[string]uint64
}

func NewStorageState() *StorageState {
	return &StorageState{
		lastBlocks: map[string]uint64{},
	}
}

func FetchStorageState(ctx context.Context, stores map[string]*state.Store) (out *StorageState, err error) {
	out = NewStorageState()
	for _, builder := range stores {
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting builder info: %w", err)
		}
		out.lastBlocks[builder.Name] = info.LastKVSavedBlock
	}
	return
}

func (s *StorageState) LastBlock(modName string) uint64 {
	return s.lastBlocks[modName]
}
