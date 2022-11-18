package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/execout"
)

type Snapshots struct {
	Completes block.Ranges // Shortest completes first, largest last.
}

func (s *Snapshots) Sort() {
	sort.Slice(s.Completes, func(i, j int) bool {
		return s.Completes[i].ExclusiveEndBlock < s.Completes[j].ExclusiveEndBlock
	})
}

func listSnapshots(ctx context.Context, config *execout.Config) (*Snapshots, error) {
	out := &Snapshots{}

	files, err := config.ListSnapshotFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	for _, file := range files {
		out.Completes = append(out.Completes, file.BlockRange)
	}
	out.Sort()
	return out, nil
}

func (s *Snapshots) LastCompleteSnapshotBefore(blockNum uint64) *block.Range {
	for i := len(s.Completes); i > 0; i-- {
		comp := s.Completes[i-1]
		if comp.ExclusiveEndBlock > blockNum {
			continue
		}
		return comp
	}
	return nil
}

func (s *Snapshots) Contains(r *block.Range) bool {
	for _, file := range s.Completes {
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
