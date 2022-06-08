package state

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/block"
)

type Snapshots struct {
	Files []Snapshot
}

func (s *Snapshots) Len() int {
	return len(s.Files)
}

func (s *Snapshots) Less(i, j int) bool {
	return s.Files[i].ExclusiveEndBlock < s.Files[j].ExclusiveEndBlock
}

func (s *Snapshots) Swap(i, j int) {
	s.Files[i], s.Files[j] = s.Files[j], s.Files[i]
}

func (s *Snapshots) LastBlock() uint64 {
	if len(s.Files) == 0 {
		return 0
	}
	return s.Files[len(s.Files)-1].ExclusiveEndBlock
}

type Snapshot struct {
	Path              string
	Range             *block.Range
	StartBlock        uint64
	ExclusiveEndBlock uint64
	Partial           bool
}

func (b *Store) ListSnapshots(ctx context.Context) (out *Snapshots, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		out = &Snapshots{}
		if err := b.Store.Walk(ctx, "", func(filename string) (err error) {
			if filename == "___store-metadata.json" || strings.HasPrefix(filename, "__") {
				return nil
			}

			fileInfo, ok := ParseFileName(filename)
			if !ok {
				return nil
			}

			out.Files = append(out.Files, Snapshot{
				Path:    filename,
				Range:   block.NewRange(fileInfo.StartBlock, fileInfo.EndBlock),
				Partial: fileInfo.Partial,
			})
			return nil
		}); err != nil {
			return fmt.Errorf("walking snapshots: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Sort(out)
	return out, nil
}
