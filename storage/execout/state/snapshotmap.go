package state

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/storage/execout"
)

type SnapshotsMap struct {
	sync.Mutex
	Snapshots map[string]*Snapshots
}

func (s *SnapshotsMap) String() string {
	var out []string
	for k, v := range s.Snapshots {
		out = append(out, fmt.Sprintf("execout=%s (%s)", k, v))
	}
	return strings.Join(out, ", ")
}

func FetchState(ctx context.Context, configs *execout.Configs) (*SnapshotsMap, error) {
	state := &SnapshotsMap{
		Snapshots: map[string]*Snapshots{},
	}

	eg := llerrgroup.New(10)

	for _, config := range configs.ConfigMap {
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
