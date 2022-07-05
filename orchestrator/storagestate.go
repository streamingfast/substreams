package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/state"
)

type StorageState struct {
	sync.Mutex
	Snapshots map[string]*Snapshots
}

func NewStorageState() *StorageState {
	return &StorageState{
		Snapshots: map[string]*Snapshots{},
	}
}

func (s *StorageState) String() string {
	var out []string
	for k, v := range s.Snapshots {
		out = append(out, fmt.Sprintf("store=%s (%s)", k, v))
	}
	return strings.Join(out, ", ")
}

func FetchStorageState(ctx context.Context, stores map[string]*state.Store) (out *StorageState, err error) {
	out = NewStorageState()
	eg := llerrgroup.New(10)
	for storeName, store := range stores {
		if eg.Stop() {
			break
		}

		objStore := store.Store
		storeName := storeName
		eg.Go(func() error {
			snapshots, err := listSnapshots(ctx, objStore)
			if err != nil {
				return err
			}
			out.Lock()
			out.Snapshots[storeName] = snapshots
			out.Unlock()
			return nil
		})
	}
	if err = eg.Wait(); err != nil {
		return nil, fmt.Errorf("running list snapshots: %w", err)
	}
	return
}
