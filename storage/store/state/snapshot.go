package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/streamingfast/substreams/storage/store"
)

func listSnapshots(ctx context.Context, storeConfig *store.Config, below uint64) (*storeSnapshots, error) {
	out := &storeSnapshots{}

	files, err := storeConfig.ListSnapshotFiles(ctx, below)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	for _, file := range files {
		if file.Partial {
			out.Partials = append(out.Partials, file)
		} else {
			out.FullKVFiles = append(out.FullKVFiles, file)
		}
	}
	out.Sort()
	return out, nil
}

type storeSnapshots struct {
	FullKVFiles store.FileInfos // Shortest FullKVs first, largest last.
	Partials    store.FileInfos // First partials first, last
}

func (s *storeSnapshots) Sort() {
	sort.SliceStable(s.FullKVFiles, func(i, j int) bool {
		return s.FullKVFiles[i].Range.ExclusiveEndBlock < s.FullKVFiles[j].Range.ExclusiveEndBlock
	})
	sort.SliceStable(s.Partials, func(i, j int) bool {
		left := s.Partials[i]
		right := s.Partials[j]

		return left.Range.StartBlock < right.Range.StartBlock
	})
}

func (s *storeSnapshots) String() string {
	return fmt.Sprintf("completes=%s, partials=%s", s.FullKVFiles, s.Partials)
}
