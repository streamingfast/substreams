package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
)

type Squasher struct {
	builders map[string]*Squashable
}

func NewSquasher(ctx context.Context, builders []*state.Builder, outputCaches map[string]*outputs.OutputCache) (*Squasher, error) {
	sqashables := map[string]*Squashable{}
	for _, builder := range builders {
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting info for %s: %w", builder.Name, err)
		}

		var initialBuilder *state.Builder
		if info.LastKVSavedBlock == 0 {
			initialBuilder = builder.FromBlockRange(nil, true)
		} else {
			r := &block.Range{
				StartBlock:        builder.ModuleStartBlock,
				ExclusiveEndBlock: info.LastKVSavedBlock,
			}
			initialBuilder = builder.FromBlockRange(r, false)
			err := initialBuilder.Initialize(ctx, r.ExclusiveEndBlock, info.RangeIntervalSize, outputCaches[builder.Name].Store)
			if err != nil {
				return nil, fmt.Errorf("initializing builder %s for range %s: %w", builder.Name, r, err)
			}
		}

		sqashables[builder.Name] = NewSquashable(initialBuilder)
	}

	return &Squasher{sqashables}, nil
}

func (s *Squasher) Squash(ctx context.Context, moduleName string, blockRange *block.Range) error {
	b, ok := s.builders[moduleName]
	if !ok {
		return nil
	}

	return s.squash(ctx, b, blockRange)
}

func (s *Squasher) squash(ctx context.Context, squashable *Squashable, blockRange *block.Range) error {
	if blockRange.Size() < squashable.builder.SaveInterval {
		return nil
	}

	squashable.ranges = append(squashable.ranges, blockRange)
	sort.Sort(squashable.ranges)

	for {
		if squashable.ranges[0].Equals(squashable.builder.BlockRange.Next(squashable.builder.SaveInterval)) {
			nextBuilder := squashable.builder.FromBlockRange(squashable.ranges[0], true)
			err := nextBuilder.Merge(squashable.builder)
			if err != nil {
				return fmt.Errorf("merging: %s", err)
			}

			err = nextBuilder.WriteState(ctx)
			if err != nil {
				return fmt.Errorf("writing state: %w", err)
			}

			squashable.builder = nextBuilder
			squashable.ranges = squashable.ranges[1:]

			continue
		}
		break
	}

	return nil
}

func (s *Squasher) Close() error {
	var nonEmptySquashables Squashables
	for _, v := range s.builders {
		if !v.IsEmpty() {
			nonEmptySquashables = append(nonEmptySquashables, v)
		}
	}

	if len(nonEmptySquashables) != 0 {
		return fmt.Errorf("squasher closed in invalid state. still waiting for %s", nonEmptySquashables)
	}

	return nil
}

type Squashable struct {
	builder *state.Builder
	ranges  block.Ranges
}

func NewSquashable(initialBuilder *state.Builder) *Squashable {
	return &Squashable{
		builder: initialBuilder,
		ranges:  block.Ranges(nil),
	}
}

func (s *Squashable) IsEmpty() bool {
	return len(s.ranges) == 0
}

func (s *Squashable) String() string {
	return fmt.Sprintf("%s: [%s]", s.builder.Name, s.ranges)
}

type Squashables []*Squashable

func (s Squashables) String() string {
	var rs []string
	for _, i := range s {
		rs = append(rs, i.String())
	}
	return strings.Join(rs, ",")
}
