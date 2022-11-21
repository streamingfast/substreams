package state

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/execout"
)

func listSnapshots(ctx context.Context, config *execout.Config) (out block.Ranges, err error) {
	files, err := config.ListSnapshotFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	for _, file := range files {
		out = append(out, file.BlockRange)
	}
	return
}
