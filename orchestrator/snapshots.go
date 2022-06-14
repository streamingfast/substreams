package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/streamingfast/derr"
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


// FIXME(abourget): rename to `LastCompleteSnapshotBeforeBlock()`
// LastCompleteSnapshotBeforeBlock returns the ExclusiveEndBlock of
// the snapshot that is the highest below the input _blockNum_.
func (s *Snapshots) LastCompletedBlockBefore(blockNum uint64) uint64 {
	for i := len(s.Completes); i > 0; i-- {
		comp := s.Completes[i-1]
		if comp.ExclusiveEndBlock > blockNum {
			continue
		}
		return comp.ExclusiveEndBlock
	}
	return 0
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

func listSnapshots(ctx context.Context, b *state.Store) (out *Snapshots, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		out = &Snapshots{}
		if err := b.Store.Walk(ctx, "", func(filename string) (err error) {
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
