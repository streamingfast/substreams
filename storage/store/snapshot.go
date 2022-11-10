package store

import (
	"context"
	"fmt"
	"sort"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/store"
)

type StoreSnapshots struct {
	Completes block.Ranges // Shortest completes first, largest last.
	Partials  block.Ranges // First partials first, last last
}

func (s *StoreSnapshots) Sort() {
	sort.Slice(s.Completes, func(i, j int) bool {
		return s.Completes[i].ExclusiveEndBlock < s.Completes[j].ExclusiveEndBlock
	})
	sort.Slice(s.Partials, func(i, j int) bool {
		return s.Partials[i].StartBlock < s.Partials[j].StartBlock
	})
}

func (s *StoreSnapshots) String() string {
	return fmt.Sprintf("completes=%s, partials=%s", s.Completes, s.Partials)
}

func (s *StoreSnapshots) LastCompletedBlock() uint64 {
	if len(s.Completes) == 0 {
		return 0
	}
	return s.Completes[len(s.Completes)-1].ExclusiveEndBlock
}

func (s *StoreSnapshots) LastCompleteSnapshotBefore(blockNum uint64) *block.Range {
	for i := len(s.Completes); i > 0; i-- {
		comp := s.Completes[i-1]
		if comp.ExclusiveEndBlock > blockNum {
			continue
		}
		return comp
	}
	return nil
}

func (s *StoreSnapshots) ContainsPartial(r *block.Range) bool {
	for _, file := range s.Partials {
		if file.StartBlock == r.StartBlock && file.ExclusiveEndBlock == r.ExclusiveEndBlock {
			return true
		}
	}
	return false
}

type Snapshot struct {
	block.Range
	Path string
}

func listSnapshots(ctx context.Context, storeConfig *store.Config) (*StoreSnapshots, error) {
	out := &StoreSnapshots{}

	files, err := storeConfig.ListSnapshotFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	for _, file := range files {
		if file.Partial {
			out.Partials = append(out.Partials, block.NewRange(file.StartBlock, file.EndBlock))
		} else {
			out.Completes = append(out.Completes, block.NewRange(file.StartBlock, file.EndBlock))
		}

	}
	out.Sort()
	return out, nil
}
