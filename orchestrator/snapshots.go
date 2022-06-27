package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
)

type Snapshots struct {
	Completes block.Ranges // Shortest completes first, largest last.
	Partials  block.Ranges // First partials first, last last
}

func (s *Snapshots) Sort() {
	sort.Slice(s.Completes, func(i, j int) bool {
		return s.Completes[i].ExclusiveEndBlock < s.Completes[j].ExclusiveEndBlock
	})
	sort.Slice(s.Partials, func(i, j int) bool {
		return s.Partials[i].StartBlock < s.Partials[j].StartBlock
	})
}

func (s *Snapshots) LastCompletedBlock() uint64 {
	if len(s.Completes) == 0 {
		return 0
	}
	return s.Completes[len(s.Completes)-1].ExclusiveEndBlock
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

func (s *Snapshots) ContainsPartial(r *block.Range) bool {
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

func listSnapshots(ctx context.Context, store dstore.Store) (*Snapshots, error) {
	out := &Snapshots{}

	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := store.Walk(ctx, "", func(filename string) (err error) {
			if filename == "___store-metadata.json" || strings.HasPrefix(filename, "__") {
				return nil
			}

			fileInfo, ok := state.ParseFileName(filename)
			if !ok {
				return nil
			}

			if fileInfo.Partial {
				out.Partials = append(out.Partials, block.NewRange(fileInfo.StartBlock, fileInfo.EndBlock))
			} else {
				out.Completes = append(out.Completes, block.NewRange(fileInfo.StartBlock, fileInfo.EndBlock))
			}
			return nil
		}); err != nil {
			return fmt.Errorf("walking snapshots: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	out.Sort()
	return out, nil
}
