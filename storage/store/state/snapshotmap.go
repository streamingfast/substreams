package state

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/storage/store"
)

type StoreSnapshotsMap struct {
	sync.Mutex
	Snapshots map[string]*storeSnapshots
}

func NewStoreSnapshotsMap() *StoreSnapshotsMap {
	return &StoreSnapshotsMap{
		Snapshots: map[string]*storeSnapshots{},
	}
}

func (s *StoreSnapshotsMap) String() string {
	var out []string
	for k, v := range s.Snapshots {
		out = append(out, fmt.Sprintf("store=%s (%s)", k, v))
	}
	return strings.Join(out, ", ")
}

// Summary reports the number of files seen per store, for logging.
func (s *StoreSnapshotsMap) Summary() string {
	var out []string
	for k, v := range s.Snapshots {
		out = append(out, fmt.Sprintf("%s: %d fullkv, %d partials", k, len(v.FullKVFiles), len(v.Partials)))
	}
	return strings.Join(out, "; ")
}

// Merge adds every file of `other` into `s`, keeping files sorted.
func (s *StoreSnapshotsMap) Merge(other *StoreSnapshotsMap) {
	for name, snapshots := range other.Snapshots {
		existing := s.Snapshots[name]
		if existing == nil {
			s.Snapshots[name] = snapshots
			continue
		}
		existing.FullKVFiles = append(existing.FullKVFiles, snapshots.FullKVFiles...)
		existing.Partials = append(existing.Partials, snapshots.Partials...)
		existing.Sort()
	}
}

func FetchState(ctx context.Context, storeConfigMap store.ConfigMap, from, to uint64) (*StoreSnapshotsMap, error) {
	state := NewStoreSnapshotsMap()

	eg := llerrgroup.New(10)

	for _, config := range storeConfigMap {
		if eg.Stop() {
			break
		}

		storeName := config.Name()
		storeConfig := config

		eg.Go(func() error {
			snapshots, err := listSnapshots(ctx, storeConfig, from, to)
			if err != nil {
				return err
			}
			state.Lock()
			state.Snapshots[storeName] = snapshots
			state.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("running list snapshots: %w", err)
	}

	return state, nil
}
