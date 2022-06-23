package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type Squashable struct {
	sync.Mutex

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

		start := time.Now()
		loadPartialStart := time.Now()
		nextStore, err := s.store.LoadFrom(ctx, block.NewRange(squashableRange.StartBlock, squashableRange.ExclusiveEndBlock))
		if err != nil {
			return fmt.Errorf("initializing next partial store %q: %w", s.name, err)
		}
		loadPartialDuration := time.Since(loadPartialStart)

		zlog.Debug("next store loaded", zap.Object("store", nextStore))

		mergeStart := time.Now()
		err = s.store.Merge(nextStore)
		if err != nil {
			return fmt.Errorf("merging: %s", err)
		}
		mergeDuration := time.Since(mergeStart)

		zlog.Debug("store merge", zap.Object("store", s.store))

		s.nextExpectedStartBlock = squashableRange.ExclusiveEndBlock

		zlog.Info("deleting store", zap.Object("store", nextStore))
		deleteStart := time.Now()

		err = nextStore.DeleteStore(ctx, squashableRange.ExclusiveEndBlock)
		if err != nil {
			zlog.Warn("deleting partial file", zap.Error(err))
		}
		deleteDuration := time.Since(deleteStart)

		writeStart := time.Now()
		if squashableRange.ExclusiveEndBlock%nextStore.SaveInterval == 0 {
			err = s.store.WriteState(ctx, squashableRange.ExclusiveEndBlock)
			if err != nil {
				return fmt.Errorf("writing state: %w", err)
			}
		}
		writeDuration := time.Since(writeStart)

		s.ranges = s.ranges[1:]

		if squashableRange.ExclusiveEndBlock == s.targetExclusiveBlock {
			s.targetReached = true
		}
		notificationStart := time.Now()
		s.notifyWaiters(squashableRange.ExclusiveEndBlock)
		notificationDuration := time.Since(notificationStart)
		totalDuration := time.Since(start)
		zlog.Info(
			"squashing completed",
			zap.String("module_name", s.name),
			zap.Stringer("squashable_range", squashableRange),
			zap.Duration("load_partial_duration", loadPartialDuration),
			zap.Duration("merge_duration", mergeDuration),
			zap.Duration("delete_duration", deleteDuration),
			zap.Duration("write_duration", writeDuration),
			zap.Duration("notification_duration", notificationDuration),
			zap.Duration("total_duration", totalDuration),
		)
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
