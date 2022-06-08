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
	reqChunk               *reqChunk
	ranges                 []*chunk // Ranges split in `storeSaveInterval` chunks
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

func (s *Squashable) squash(ctx context.Context, reqChunk *reqChunk) error {
	s.Lock()
	defer s.Unlock()

	zlog.Info("cumulating squash request range", zap.String("module", s.name), zap.Stringer("req_chunk", reqChunk))

	s.ranges = append(s.ranges, reqChunk.chunks...)
	sort.Slice(s.ranges, func(i, j int) bool {
		return s.ranges[i].start < s.ranges[j].start
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

		if squashableRange.start < s.nextExpectedStartBlock {
			return fmt.Errorf("module %q: non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.name, s.nextExpectedStartBlock, squashableRange.start, s.ranges)
		}
		if s.nextExpectedStartBlock != squashableRange.start {
			break
		}

		zlog.Debug("found range to merge", zap.Stringer("squashable", s), zap.Stringer("squashable_range", squashableRange))

		nextStore, err := s.store.LoadFrom(ctx, block.NewRange(squashableRange.start, squashableRange.end))
		if err != nil {
			return fmt.Errorf("initializing next partial store %q: %w", s.name, err)
		}

		err = s.store.Merge(nextStore)
		if err != nil {
			return fmt.Errorf("merging: %s", err)
		}

		s.nextExpectedStartBlock = squashableRange.end

		if squashableRange.tempPartial {
			err = nextStore.DeleteStore(ctx, squashableRange.end)
			if err != nil {
				zlog.Warn("deleting partial file", zap.Error(err))
			}
		} else {
			err = s.store.WriteState(ctx, squashableRange.end)
			if err != nil {
				return fmt.Errorf("writing state: %w", err)
			}
		}

		s.ranges = s.ranges[1:]

		if squashableRange.end == s.targetExclusiveBlock {
			s.targetReached = true
		}

		s.notifyWaiters(squashableRange.end)
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
