package state

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/streamingfast/substreams/storage/store"
)

func listSnapshots(ctx context.Context, storeConfig *store.Config, from, to uint64) (*storeSnapshots, error) {
	out := &storeSnapshots{}

	files, err := storeConfig.ListSnapshotFiles(ctx, from, &to)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	for _, file := range files {
		if file.Partial {
			out.Partials = append(out.Partials, file)
			continue
		}
		// A full KV is only loadable under the name built from the store's own initial block:
		// files written under another one (ex: before the first streamable block was raised)
		// would be counted as present but fail to load.
		if file.Range.StartBlock != storeConfig.ModuleInitialBlock() {
			continue
		}
		out.FullKVFiles = append(out.FullKVFiles, file)
	}
	out.Sort()
	return out, nil
}

type storeSnapshots struct {
	FullKVFiles store.FileInfos // Shortest FullKVs first, largest last.
	Partials    store.FileInfos // First partials first, last
}

func (s *storeSnapshots) Sort() {
	slices.SortStableFunc(s.FullKVFiles, func(a, b *store.FileInfo) int {
		return cmp.Compare(a.Range.ExclusiveEndBlock, b.Range.ExclusiveEndBlock)
	})
	slices.SortStableFunc(s.Partials, func(a, b *store.FileInfo) int {
		return cmp.Compare(a.Range.StartBlock, b.Range.StartBlock)
	})
}

func (s *storeSnapshots) String() string {
	return fmt.Sprintf("completes=%s, partials=%s", s.FullKVFiles, s.Partials)
}
