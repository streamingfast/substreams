package state

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/streamingfast/substreams/block"

	"github.com/streamingfast/substreams/storage/execout"
)

type SnapshotsMap struct {
	sync.Mutex
	Snapshots map[string]block.Ranges
}

func (s *SnapshotsMap) String() string {
	var out []string
	for k, v := range s.Snapshots {
		out = append(out, fmt.Sprintf("execout=%s (%s)", k, v))
	}
	return strings.Join(out, ", ")
}

func FetchMappersState(ctx context.Context, configs *execout.Configs, outputModule string) (*SnapshotsMap, error) {

	config := configs.ConfigMap[outputModule]

	if config == nil {
		return nil, nil
	}

	snapshots, err := listSnapshots(ctx, config)
	if err != nil {
		return nil, err
	}

	return &SnapshotsMap{
		Snapshots: map[string]block.Ranges{outputModule: snapshots},
	}, nil
}
