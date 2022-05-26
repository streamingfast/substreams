package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
)

type Squasher struct {
	squashables       map[string]*Squashable
	storeSaveInterval uint64

	notifier Notifier

	lock              sync.Mutex
}

type SquasherOption func(s *Squasher)

func WithNotifier(notifier Notifier) SquasherOption {
	return func(s *Squasher) {
		s.notifier = notifier
	}
}

func NewSquasher(ctx context.Context, builders []*state.Builder, outputCaches map[string]*outputs.OutputCache, storeSaveInterval uint64, opts ...SquasherOption) (*Squasher, error) {
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

	squasher := &Squasher{squashables: squashables, storeSaveInterval: storeSaveInterval}

	for _, opt := range opts {
		opt(squasher)
	}

	return squasher, nil
}

func (s *Squasher) Squash(ctx context.Context, moduleName string, requestBlockRange *block.Range) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	squashable, ok := s.squashables[moduleName]
	if !ok {
		panic(fmt.Sprintf("invalid module %q", moduleName))
		return nil
	}
	builder := squashable.builder

	zlog.Info("squashing request range", zap.String("module", builder.Name), zap.Object("request_range", requestBlockRange))
	blockRanges := requestBlockRange.Split(s.storeSaveInterval)

	for _, br := range blockRanges {
		zlog.Info("squashing range", zap.String("module", builder.Name), zap.Object("range", br))
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

		isNotFullRange := br.Size() < squashable.builder.SaveInterval && br.StartBlock != squashable.builder.ModuleStartBlock
		if isNotFullRange {
			continue
		}

		err := squash(ctx, squashable, br, s.notifier)
		if err != nil {
			return fmt.Errorf("squashing range %d-%d: %w", br.StartBlock, br.ExclusiveEndBlock, err)
		}
	}

	return nil
}

func squash(ctx context.Context, squashable *Squashable, blockRange *block.Range, notifier Notifier) error {
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

		squashableRange := squashable.ranges[0]
		zlog.Info("checking if builder squashable", zap.Object("current_builder_range", squashable.builder.BlockRange), zap.Object("next_available_squashable_range", squashableRange))

		if squashable.builder.BlockRange.IsNext(squashableRange, squashable.builder.SaveInterval) {
			zlog.Debug("found range to merge", zap.String("squashable", squashable.String()))

			partialBuilder := squashable.builder.FromBlockRange(squashableRange, true)
			err := partialBuilder.InitializePartial(ctx, squashableRange.StartBlock)
			if err != nil {
				return fmt.Errorf("initializing next partial builder %q: %w", partialBuilder.Name, err)
			}

			err = squashable.builder.Merge(partialBuilder)
			if err != nil {
				return fmt.Errorf("merging: %s", err)
			}

			err = partialBuilder.DeletePartialFile(ctx)
			if err != nil {
				return fmt.Errorf("deleting builder data: %w", err)
			}

			err = squashable.builder.WriteState(ctx)
			if err != nil {
				return fmt.Errorf("writing state: %w", err)
			}

			squashable.ranges = squashable.ranges[1:]

			if notifier != nil {
				notifier.Notify(squashable.builder.Name, squashableRange.ExclusiveEndBlock)
			}

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
