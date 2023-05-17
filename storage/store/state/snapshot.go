package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
)

type storeSnapshots struct {
	Completes store.FileInfos // Shortest completes first, largest last.
	Partials  store.FileInfos // First partials first, last
}

func (s *storeSnapshots) Sort() {
	sort.SliceStable(s.Completes, func(i, j int) bool {
		return s.Completes[i].Range.ExclusiveEndBlock < s.Completes[j].Range.ExclusiveEndBlock
	})
	sort.SliceStable(s.Partials, func(i, j int) bool {
		left := s.Partials[i]
		right := s.Partials[j]

		// Sort by start block first, then by trace ID so at least we
		// take partials all from the same producer.
		if left.Range.StartBlock == right.Range.StartBlock {
			return left.TraceID < right.TraceID
		}

		return left.Range.StartBlock < right.Range.StartBlock
	})
}

func (s *storeSnapshots) String() string {
	return fmt.Sprintf("completes=%s, partials=%s", s.Completes, s.Partials)
}

func (s *storeSnapshots) LastCompletedBlock() uint64 {
	if len(s.Completes) == 0 {
		return 0
	}
	return s.Completes[len(s.Completes)-1].Range.ExclusiveEndBlock
}

func (s *storeSnapshots) LastCompleteSnapshotBefore(blockNum uint64) *store.FileInfo {
	for i := len(s.Completes); i > 0; i-- {
		comp := s.Completes[i-1]
		if comp.Range.ExclusiveEndBlock > blockNum {
			continue
		}
		return comp
	}
	return nil
}

// findPartial returns the partial file that matches the given range, or nil if none matches.
func (s *storeSnapshots) findPartial(r *block.Range) *store.FileInfo {
	for _, file := range s.Partials {
		if r.Equals(file.Range) {
			return file
		}
	}
	return nil
}

type Snapshot struct {
	block.Range
	Path string
}

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
			out.Completes = append(out.Completes, file)
		}
	}
	out.Sort()
	return out, nil
}
