package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
)

type Squasher struct {
	builders map[string]*Squashable
}

func NewSquasher(ctx context.Context, partialRequest *pbsubstreams.Request, builders []*state.Builder, outputCaches map[string]*outputs.OutputCache) (*Squasher, error) {
	squashables := map[string]*Squashable{}
	for _, builder := range builders {
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting info for %s: %w", builder.Name, err)
		}

		var initialBuilder *state.Builder
		if info.LastKVSavedBlock == 0 {
			r := &block.Range{
				StartBlock:        builder.ModuleStartBlock,
				ExclusiveEndBlock: builder.ModuleStartBlock + builder.SaveInterval,
			}
			initialBuilder = builder.FromBlockRange(r, true)
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

		squashables[builder.Name] = NewSquashable(initialBuilder)
	}

	return &Squasher{squashables}, nil
}

func (s *Squasher) Squash(ctx context.Context, moduleName string, blockRange *block.Range) error {
	b, ok := s.builders[moduleName]
	if !ok {
		return nil
	}

	blockRanges := blockRange.Split(100)

	for _, br := range blockRanges {
		err := squash(ctx, b, br)
		if err != nil {
			return fmt.Errorf("squashing range %d-%d: %w", br.StartBlock, br.ExclusiveEndBlock, err)
		}
	}

	return nil
}

func squash(ctx context.Context, squashable *Squashable, blockRange *block.Range) error {
	zlog.Info("squashing", zap.Object("range", blockRange), zap.String("module_name", squashable.builder.Name), zap.Uint64("block_range_size", blockRange.Size()))
	if blockRange.Size() < squashable.builder.SaveInterval {
		return nil
	}

	// add this range to the squashable's range list, and sort them in ascending order
	squashable.ranges = append(squashable.ranges, blockRange)
	sort.Sort(squashable.ranges)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			//
		}

		if len(squashable.ranges) == 0 {
			zlog.Debug("all available squashable ranges have been merged", zap.String("squashable", squashable.String()))
			break
		}

		nextAvailableSquashableRange := squashable.ranges[0]
		nextBuilderRange := squashable.builder.BlockRange.Next(squashable.builder.SaveInterval)

		if nextAvailableSquashableRange.Equals(nextBuilderRange) {
			zlog.Debug("found range to merge", zap.String("squashable", squashable.String()), zap.String("mergeable range", nextBuilderRange.String()))

			nextBuilder := squashable.builder.FromBlockRange(nextAvailableSquashableRange, true)
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
