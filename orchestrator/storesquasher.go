package orchestrator

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/metrics"
	"sort"
	"time"

	"github.com/abourget/llerrgroup"
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
	log                     *zap.Logger

	jobsPlanner *JobsPlanner

	targetExclusiveEndBlockReach bool
	partialsChunks               chan block.Ranges
	waitForCompletion            chan interface{}
}

func NewStoreSquasher(initialStore *state.Store, targetExclusiveBlock, nextExpectedStartBlock uint64, jobsPlanner *JobsPlanner) *StoreSquasher {
	s := &StoreSquasher{
		Shutter:                 shutter.New(),
		name:                    initialStore.Name,
		store:                   initialStore,
		targetExclusiveEndBlock: targetExclusiveBlock,
		nextExpectedStartBlock:  nextExpectedStartBlock,
		jobsPlanner:             jobsPlanner,
		partialsChunks:          make(chan block.Ranges, 100 /* before buffering the upstream requests? */),
		waitForCompletion:       make(chan interface{}),
		log:                     zlog.With(zap.String("module", initialStore.Name), zap.String("module_hash", initialStore.ModuleHash)),
	}
	s.OnTerminating(func(err error) {
		if err != nil {
			s.log.Info("squasher terminating because of an error", zap.Error(err))
			return
		}
		s.log.Info("will terminate after partials chucks chan empty")
		close(s.partialsChunks)

		s.log.Info("waiting completion")
		<-s.waitForCompletion
		s.log.Info("partials chucks chan empty, terminating")
	})
	return s
}

func (s *StoreSquasher) squash(partialsChunks block.Ranges) error {
	if len(partialsChunks) == 0 {
		return fmt.Errorf("partialsChunks is empty for module %q", s.name)
	}

	s.log.Info("cumulating squash request range", zap.Stringer("req_chunk", partialsChunks))
	s.partialsChunks <- partialsChunks
	return nil
}

func (s *StoreSquasher) launch(ctx context.Context) {
	zlog.Info("launching squasher", zap.String("module_name", s.store.Name))
	metrics.SquashesLaunched.Inc()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("quitting on a close context")
			return

		case partialsChunks, ok := <-s.partialsChunks:
			if !ok {
				s.log.Info("squashing done, no more partial chunks to squash")
				close(s.waitForCompletion)
				return
			}
			s.log.Info("got partials chunks", zap.Stringer("partials_chunks", partialsChunks))
			s.ranges = append(s.ranges, partialsChunks...)
			sort.Slice(s.ranges, func(i, j int) bool {
				return s.ranges[i].StartBlock < s.ranges[j].ExclusiveEndBlock
			})
		}

		eg := llerrgroup.New(250)
		start := time.Now()
		squashCount := 0
		var lastExclusiveEndBlock uint64
		for {
			if eg.Stop() {
				break
			}

			if len(s.ranges) == 0 {
				s.log.Info("no more ranges to squash")
				break
			}
			squashableRange := s.ranges[0]
			s.log.Info("testing first range", zap.Object("range", squashableRange), zap.Uint64("next_expected_start_block", s.nextExpectedStartBlock))

			if squashableRange.StartBlock < s.nextExpectedStartBlock {
				s.Shutdown(fmt.Errorf("module %q: non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.name, s.nextExpectedStartBlock, squashableRange.StartBlock, s.ranges))
				return
			}
			if s.nextExpectedStartBlock != squashableRange.StartBlock {
				break
			}

			s.log.Debug("found range to merge", zap.Stringer("squashable", s), zap.Stringer("squashable_range", squashableRange))
			squashCount++

			nextStore, err := s.store.LoadFrom(ctx, block.NewRange(squashableRange.StartBlock, squashableRange.ExclusiveEndBlock))
			if err != nil {
				s.Shutdown(fmt.Errorf("initializing next partial store %q: %w", s.name, err))
				return
			}

			s.log.Debug("next store loaded", zap.Object("store", nextStore))

			err = s.store.Merge(nextStore)
			if err != nil {
				s.Shutdown(fmt.Errorf("merging: %s", err))
				return
			}

			zlog.Debug("store merge", zap.Object("store", s.store))

			s.nextExpectedStartBlock = squashableRange.ExclusiveEndBlock

			zlog.Info("deleting store", zap.Object("store", nextStore))

			storeDeleter := nextStore.DeleteStore(ctx, squashableRange.ExclusiveEndBlock)
			eg.Go(storeDeleter.Delete)

			isSaveIntervalReached := squashableRange.ExclusiveEndBlock%nextStore.SaveInterval == 0
			isFirstKvForModule := isSaveIntervalReached && squashableRange.StartBlock == s.store.ModuleInitialBlock
			isCompletedKv := isSaveIntervalReached && squashableRange.Len()-s.store.SaveInterval == 0
			zlog.Info("should write store?", zap.Uint64("exclusiveEndBlock", squashableRange.ExclusiveEndBlock), zap.Uint64("save_interval", nextStore.SaveInterval), zap.Bool("is_save_interval_reached", isSaveIntervalReached), zap.Bool("is_first_kv_for_module", isFirstKvForModule), zap.Bool("is_completed_kv", isCompletedKv))
			if isFirstKvForModule || isCompletedKv {
				storeWriter, err := s.store.WriteState(ctx, squashableRange.ExclusiveEndBlock)
				if err != nil {
					s.Shutdown(fmt.Errorf("store writer marshaling: %w", err))
					return
				}
				eg.Go(storeWriter.Write)
			}

			s.ranges = s.ranges[1:]

			if squashableRange.ExclusiveEndBlock == s.targetExclusiveEndBlock {
				s.targetExclusiveEndBlockReach = true
			}
			s.log.Debug("signaling the jobs planner that we completed", zap.String("module", s.name), zap.Uint64("end_block", squashableRange.ExclusiveEndBlock))
			lastExclusiveEndBlock = squashableRange.ExclusiveEndBlock
		}
		s.log.Info("waiting for eg to finish")
		if err := eg.Wait(); err != nil {
			// eg.Wait() will block until everything is done, and return the first error.
			s.Shutdown(err)
			return
		}

		if lastExclusiveEndBlock != 0 {
			s.jobsPlanner.SignalCompletionUpUntil(s.name, lastExclusiveEndBlock)
		}

		totalDuration := time.Since(start)
		avgDuration := time.Duration(0)
		if squashCount > 0 {
			avgDuration = totalDuration / time.Duration(squashCount)
		}
		metrics.LastSquashDuration.SetUint64(uint64(totalDuration))
		metrics.LastSquashAvgDuration.SetUint64(uint64(avgDuration))
		zlog.Info("squashing done", zap.String("module_name", s.store.Name), zap.Duration("duration", totalDuration), zap.Duration("squash_avg", avgDuration))
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
