package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type Squashable struct {
	sync.RWMutex

	name                   string
	store                  *state.Store
	requestRange           *block.Range
	ranges                 block.Ranges
	targetExclusiveBlock   uint64
	nextExpectedStartBlock uint64

	notifier Notifier

	targetReached bool
}

func NewSquashable(initialStore *state.Store, targetExclusiveBlock, nextExpectedStartBlock uint64, notifier Notifier) *Squashable {
	return &Squashable{
		name:                   initialStore.Name,
		store:                  initialStore,
		targetExclusiveBlock:   targetExclusiveBlock,
		nextExpectedStartBlock: nextExpectedStartBlock,
		notifier:               notifier,
	}
}

func (s *Squashable) squash(ctx context.Context, partialsChunks block.Ranges) error {
	s.Lock()
	defer s.Unlock()

	zlog.Info("cumulating squash request range", zap.String("module", s.name), zap.Stringer("req_chunk", partialsChunks))

	s.ranges = append(s.ranges, partialsChunks...)
	sort.Slice(s.ranges, func(i, j int) bool {
		return s.ranges[i].StartBlock < s.ranges[j].ExclusiveEndBlock
	})

	if err := s.mergeAvailablePartials(ctx); err != nil {
		return fmt.Errorf("merging partials: %w", err)
	}

	return nil
}

func (s *Squashable) mergeAvailablePartials(ctx context.Context) error {
	zlog.Info("squashing", zap.String("module_name", s.store.Name))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if len(s.ranges) == 0 {
			break
		}

		// TODO(abourget): we still need to keep track of which ranges were completed in order
		squashableRange := s.ranges[0]
		zlog.Info("testing first range", zap.String("module_name", s.store.Name), zap.Object("range", squashableRange), zap.Uint64("next_expected_start_block", s.nextExpectedStartBlock))

		if squashableRange.StartBlock < s.nextExpectedStartBlock {
			return fmt.Errorf("module %q: non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.name, s.nextExpectedStartBlock, squashableRange.StartBlock, s.ranges)
		}
		if s.nextExpectedStartBlock != squashableRange.StartBlock {
			break
		}

		zlog.Debug("found range to merge", zap.Stringer("squashable", s), zap.Stringer("squashable_range", squashableRange))

		nextStore, err := s.store.LoadFrom(ctx, block.NewRange(squashableRange.StartBlock, squashableRange.ExclusiveEndBlock))
		if err != nil {
			return fmt.Errorf("initializing next partial store %q: %w", s.name, err)
		}

		zlog.Debug("next store loaded", zap.Object("store", nextStore))

		err = s.store.Merge(nextStore)
		if err != nil {
			return fmt.Errorf("merging: %s", err)
		}

		zlog.Debug("store merge", zap.Object("store", s.store))

		s.nextExpectedStartBlock = squashableRange.ExclusiveEndBlock
		//s.store.BlockRange.ExclusiveEndBlock = nextStore.BlockRange.ExclusiveEndBlock

		if squashableRange.ExclusiveEndBlock%nextStore.SaveInterval != 0 {
			//if squashableRange.tempPartial {
			zlog.Info("deleting temp store", zap.Object("store", nextStore))
			err = nextStore.DeleteStore(ctx, squashableRange.ExclusiveEndBlock)
			if err != nil {
				zlog.Warn("deleting partial file", zap.Error(err))
			}
		} else {
			err = s.store.WriteState(ctx, squashableRange.ExclusiveEndBlock)
			if err != nil {
				return fmt.Errorf("writing state: %w", err)
			}
		}

		s.ranges = s.ranges[1:]

		if squashableRange.ExclusiveEndBlock == s.targetExclusiveBlock {
			s.targetReached = true
		}

		s.notifyWaiters(squashableRange.ExclusiveEndBlock)
	}

	return nil
}

func (s *Squashable) notifyWaiters(lastSquashedBlock uint64) {
	if s.notifier != nil {
		s.notifier.Notify(s.store.Name, lastSquashedBlock)
	}
}

func (s *Squashable) IsEmpty() bool {
	return len(s.ranges) == 0
}

func (s *Squashable) String() string {
	var add string
	if s.targetReached {
		add = " (target reached)"
	}
	return fmt.Sprintf("%s%s: [%s]", s.name, add, s.ranges)
}

type Squashables []*Squashable

func (s Squashables) String() string {
	var rs []string
	for _, i := range s {
		rs = append(rs, i.String())
	}
	return strings.Join(rs, ", ")
}
