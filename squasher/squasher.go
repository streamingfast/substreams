package squasher

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
	"sort"
	"strings"
)

type Squashable struct {
	builder *state.Builder
	ranges  block.Ranges
}

func (s *Squashable) isEmpty() bool {
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

type Squasher struct {
	builders map[string]*Squashable
}

func (s *Squasher) Squash(ctx context.Context, moduleName string, blockRange *block.Range) error {
	b, ok := s.builders[moduleName]
	if !ok {
		return nil
	}

	return s.squash(ctx, b, blockRange)
}

func (s *Squasher) squash(ctx context.Context, o *Squashable, blockRange *block.Range) error {
	if blockRange.Size() < o.builder.SaveInterval {
		return nil
	}

	o.ranges = append(o.ranges, blockRange)
	sort.Sort(o.ranges)

	for {
		if o.ranges[0].Equals(o.builder.BlockRange.Next(o.builder.SaveInterval)) {
			nextBuilder := o.builder.FromBlockRange(o.ranges[0], true)
			err := nextBuilder.Merge(o.builder)
			if err != nil {
				return fmt.Errorf("merging: %s", err)
			}

			err = nextBuilder.WriteState(ctx)
			if err != nil {
				return fmt.Errorf("writing state: %w", err)
			}

			o.builder = nextBuilder
			o.ranges = o.ranges[1:]

			continue
		}
		break
	}

	return nil
}

func (s *Squasher) Close() error {
	var nonEmptySquashables Squashables
	for _, v := range s.builders {
		if !v.isEmpty() {
			nonEmptySquashables = append(nonEmptySquashables, v)
		}
	}

	if len(nonEmptySquashables) != 0 {
		return fmt.Errorf("squasher closed in invalid state. still waiting for %s", nonEmptySquashables)
	}

	return nil
}
