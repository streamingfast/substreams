package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
)

type Squasher struct {
	squashables map[string]*Squashable
}

func NewSquasher(ctx context.Context, builders []*state.Builder, outputCaches map[string]*outputs.OutputCache) (*Squasher, error) {
	squashables := map[string]*Squashable{}
	for _, builder := range builders {
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting info for %s: %w", builder.Name, err)
		}

		var initialBuilder *state.Builder
		if info.LastKVSavedBlock == 0 {
			floor := builder.ModuleStartBlock - builder.ModuleStartBlock%builder.SaveInterval
			r := &block.Range{
				StartBlock:        builder.ModuleStartBlock,
				ExclusiveEndBlock: floor + builder.SaveInterval,
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

	return &Squasher{squashables: squashables}, nil
}

func (s *Squasher) Squash(ctx context.Context, moduleName string, requestBlockRange *block.Range) error {
	squashable, ok := s.squashables[moduleName]
	if !ok {
		//should panic here
		return nil
	}
	builder := squashable.builder

	blockRanges := requestBlockRange.Split(100)

	for _, br := range blockRanges {
		if !builder.Initialized && builder.PartialMode && br.StartBlock == builder.ModuleStartBlock {
			err := builder.InitializePartial(ctx, builder.ModuleStartBlock)
			if err != nil {
				return fmt.Errorf("initializing partial builder %q on first block range: %w", builder.Name, err)
			}
			builder.PartialMode = false //this is a hack!
			err = builder.WriteState(ctx)
			if err != nil {
				return fmt.Errorf("write initial state for builder %q on first block range: %w", builder.Name, err)
			}
			continue
		}

		err := squash(ctx, squashable, br)
		if err != nil {
			return fmt.Errorf("squashing range %d-%d: %w", br.StartBlock, br.ExclusiveEndBlock, err)
		}
	}

	return nil
}

func squash(ctx context.Context, squashable *Squashable, blockRange *block.Range) error {
	zlog.Info("squashing", zap.Object("range", blockRange), zap.String("module_name", squashable.builder.Name), zap.Uint64("block_range_size", blockRange.Size()))

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

		//706-800 <- 800-900
		//1000-1100 <- 1100-1200
		nextAvailableSquashableRange := squashable.ranges[0]
		nextBuilderRange := squashable.builder.BlockRange.Next(squashable.builder.SaveInterval)
		zlog.Debug("checking if builder squashable", zap.Object("current_builder_range", squashable.builder.BlockRange), zap.Object("next_builder_range", nextBuilderRange), zap.Object("next_available_squashable_range", nextAvailableSquashableRange))

		if nextAvailableSquashableRange.Equals(nextBuilderRange) {
			zlog.Debug("found range to merge", zap.String("squashable", squashable.String()), zap.String("mergeable range", nextBuilderRange.String()))

			//nextAvailableSquashableRange.StartBlock = squashable.builder.ModuleStartBlock
			nextBuilder := squashable.builder.FromBlockRange(nextAvailableSquashableRange, true)
			zlog.Debug("WTF")
			err := nextBuilder.InitializePartial(ctx, nextAvailableSquashableRange.StartBlock)
			if err != nil {
				return fmt.Errorf("initializing next partial builder %q: %w", nextBuilder.Name, err)
			}

			err = nextBuilder.Merge(squashable.builder)
			if err != nil {
				return fmt.Errorf("merging: %s", err)
			}
			//this is very weird stuff
			nextBuilder.PartialMode = false
			nextBuilder.BlockRange.StartBlock = nextBuilder.ModuleStartBlock
			//nextBuilder.Roll()

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
	for _, v := range s.squashables {
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
