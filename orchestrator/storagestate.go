package orchestrator

import (
	"context"
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
)

type StorageState struct {
	initialBlocks map[string]uint64
	lastBlocks    map[string]uint64
}

func NewStorageState() *StorageState {
	return &StorageState{
		lastBlocks:    map[string]uint64{},
		initialBlocks: map[string]uint64{},
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
		out.initialBlocks[builder.Name] = builder.StoreInitialBlock
	}
	return
}

func (s *StorageState) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, lastBlock := range s.lastBlocks {
		if lastBlock == 0 {
			continue
		}
		initial := s.initialBlocks[storeName]
		out = append(out, &pbsubstreams.ModuleProgress{
			Name: storeName,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: initial,
							EndBlock:   lastBlock,
						},
					},
				},
			},
		})
	}
	return
}
