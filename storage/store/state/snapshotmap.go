package state

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/storage/store"
)

type storeSnapshotsMap struct {
	sync.Mutex
	Snapshots map[string]*storeSnapshots
}

func (s *storeSnapshotsMap) String() string {
	var out []string
	for k, v := range s.Snapshots {
		out = append(out, fmt.Sprintf("store=%s (%s)", k, v))
	}
	return strings.Join(out, ", ")
}

func FetchState(ctx context.Context, storeConfigMap store.ConfigMap) (*storeSnapshotsMap, error) {
	state := &storeSnapshotsMap{
		Snapshots: map[string]*storeSnapshots{},
	}

	eg := llerrgroup.New(10)

	for _, config := range storeConfigMap {
		if eg.Stop() {
			break
		}

		storeName := config.Name()
		storeConfig := config

		eg.Go(func() error {
			snapshots, err := listSnapshots(ctx, storeConfig)
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
