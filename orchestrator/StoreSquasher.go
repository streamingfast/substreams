package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

type StoreSquasher struct {
	*shutter.Shutter
	name                    string
	store                   *state.Store
	requestRange            *block.Range
	ranges                  block.Ranges
	targetStartBlock        uint64
	targetExclusiveEndBlock uint64
	nextExpectedStartBlock  uint64

	notifier Notifier

	targetExclusiveEndBlockReach bool
	partialsChunks               chan block.Ranges
	waitForCompletion            chan interface{}
}

func NewStoreSquasher(initialStore *state.Store, targetExclusiveBlock, nextExpectedStartBlock uint64, notifier Notifier) *StoreSquasher {
	s := &StoreSquasher{
		Shutter:                 shutter.New(),
		name:                    initialStore.Name,
		store:                   initialStore,
		targetExclusiveEndBlock: targetExclusiveBlock,
		nextExpectedStartBlock:  nextExpectedStartBlock,
		notifier:                notifier,
		partialsChunks:          make(chan block.Ranges, 1000),
		waitForCompletion:       make(chan interface{}),
	}
	s.OnTerminating(func(err error) {
		if err != nil {
			zlog.Info("squasher terminating because of an error", zap.String("module", s.name), zap.Error(err))
			return
		}
		zlog.Info("will terminate after partials chucks chan empty", zap.String("module", s.name))
		close(s.partialsChunks)
		<-s.waitForCompletion
		zlog.Info("partials chucks chan empty, terminating", zap.String("module", s.name))
	})
	return s
}

func (s *StoreSquasher) squash(partialsChunks block.Ranges) error {
	if len(partialsChunks) == 0 {
		panic("partialsChunks is empty")
	}

	zlog.Info("cumulating squash request range", zap.String("module", s.name), zap.Stringer("req_chunk", partialsChunks))

	s.partialsChunks <- partialsChunks
	return nil
}

func (s *StoreSquasher) launch(ctx context.Context) {
	zlog.Info("launching squasher", zap.String("module_name", s.store.Name))

waitForPartials:
	for {
		select {
		case <-ctx.Done():
			return
		case partialsChunks, ok := <-s.partialsChunks:
			if !ok {
				zlog.Info("squashing done, no more partial chuck to squash", zap.String("module_name", s.store.Name))
				close(s.waitForCompletion)
				return
			}
			s.ranges = append(s.ranges, partialsChunks...)
			sort.Slice(s.ranges, func(i, j int) bool {
				return s.ranges[i].StartBlock < s.ranges[j].ExclusiveEndBlock
			})
		}

		for {
			if len(s.ranges) == 0 {
				zlog.Info("no more ranges to squash", zap.String("module_name", s.store.Name))
				break waitForPartials
			}
			squashableRange := s.ranges[0]
			zlog.Info("testing first range", zap.String("module_name", s.store.Name), zap.Object("range", squashableRange), zap.Uint64("next_expected_start_block", s.nextExpectedStartBlock))

			if squashableRange.StartBlock < s.nextExpectedStartBlock {
				s.Shutdown(fmt.Errorf("module %q: non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.name, s.nextExpectedStartBlock, squashableRange.StartBlock, s.ranges))
				return
			}
			if s.nextExpectedStartBlock != squashableRange.StartBlock {
				continue waitForPartials
			}

			zlog.Debug("found range to merge", zap.Stringer("squashable", s), zap.Stringer("squashable_range", squashableRange))

			start := time.Now()
			loadPartialStart := time.Now()
			nextStore, err := s.store.LoadFrom(ctx, block.NewRange(squashableRange.StartBlock, squashableRange.ExclusiveEndBlock))
			if err != nil {
				s.Shutdown(fmt.Errorf("initializing next partial store %q: %w", s.name, err))
				return
			}
			loadPartialDuration := time.Since(loadPartialStart)

			zlog.Debug("next store loaded", zap.Object("store", nextStore))

			mergeStart := time.Now()
			err = s.store.Merge(nextStore)
			if err != nil {
				s.Shutdown(fmt.Errorf("merging: %s", err))
				return
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
					s.Shutdown(fmt.Errorf("writing state: %w", err))
					return
				}
			}
			writeDuration := time.Since(writeStart)

			s.ranges = s.ranges[1:]

			if squashableRange.ExclusiveEndBlock == s.targetExclusiveEndBlock {
				s.targetExclusiveEndBlockReach = true
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
	}

	return
}

func (s *StoreSquasher) notifyWaiters(lastSquashedBlock uint64) {
	if s.notifier != nil {
		s.notifier.Notify(s.store.Name, lastSquashedBlock)
	}
}

func (s *StoreSquasher) IsEmpty() bool {
	return len(s.ranges) == 0
}

func (s *StoreSquasher) String() string {
	var add string
	if s.targetExclusiveEndBlockReach {
		add = " (target reached)"
	}
	return fmt.Sprintf("%s%s: [%s]", s.name, add, s.ranges)
}

type Squashables []*StoreSquasher

func (s Squashables) String() string {
	var rs []string
	for _, i := range s {
		rs = append(rs, i.String())
	}
	return strings.Join(rs, ", ")
}
