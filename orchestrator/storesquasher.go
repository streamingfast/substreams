package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"github.com/streamingfast/substreams/metrics"
	"sort"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

type StoreSquasher struct {
	name                         string
	store                        *store.FullKV
	requestRange                 *block.Range
	ranges                       block.Ranges
	targetStartBlock             uint64  // FIXME: THIS ISN'T USED
	targetExclusiveEndBlock      uint64 // FIXME: The value we receive in this is the `request.EffectiveStartBlock()` .. and its received as `targetExclusiveBlock` and assigned to `targetExclusiveEndBlock`. What's happening?
	nextExpectedStartBlock       uint64
	log                          *zap.Logger
	jobsPlanner                  *JobsPlanner
	targetExclusiveEndBlockReach bool
	partialsChunks               chan block.Ranges
	waitForCompletion            chan error
	storeSaveInterval            uint64
}

func NewStoreSquasher(
	initialStore *store.FullKV,
	targetExclusiveBlock,
	nextExpectedStartBlock uint64,
	storeSaveInterval uint64,
	jobsPlanner *JobsPlanner,
) *StoreSquasher {
	s := &StoreSquasher{
		name:                    initialStore.Name(),
		store:                   initialStore,
		targetExclusiveEndBlock: targetExclusiveBlock,
		nextExpectedStartBlock:  nextExpectedStartBlock,
		jobsPlanner:             jobsPlanner,
		storeSaveInterval:       storeSaveInterval,
		partialsChunks:          make(chan block.Ranges, 100 /* before buffering the upstream requests? */),
		waitForCompletion:       make(chan error),
		log:                     zlog.With(zap.Object("initial_store", initialStore)),
	}
	return s
}

func (s *StoreSquasher) WaitForCompletion(ctx context.Context) error {
	// TODO(abourget): unsure what this line means, a `close()` doesn't wait?
	s.log.Info("waiting form terminate after partials chucks chan empty")
	close(s.partialsChunks)

	s.log.Info("waiting for completion")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-s.waitForCompletion:
		if err != nil {
			return fmt.Errorf("store squasher waiting for completion: %w", err)
		}
		s.log.Info("squasher completed")
		return nil
	}
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
	s.log.Info("launching store squasher")
	metrics.SquashProcessesLaunched.Inc()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("quitting on a close context")
			// TODO(abourget): don't write anything here?
			// the goroutine waiting on this one will ALSO trigger on `ctx.Done()`
			// and exit properly... so no need to clog that channel.
			s.waitForCompletion <- ctx.Err()
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

		out, err := s.processRanges(ctx, eg)
		if err != nil {
			s.waitForCompletion <- err
			// FIXME (abourget): shouldn't we return here?
			// there are risks we're not waiting for all threads in `eg.Wait()`, and
			// another risk that we block upon writing to `s.waitForCompletion` 5 lines
			// below.
			return
		}

		s.log.Info("waiting for eg to finish")
		if err := eg.Wait(); err != nil {
			// eg.Wait() will block until everything is done, and return the first error.
			s.waitForCompletion <- fmt.Errorf("waiting: %w", err)
			return
		}

		if out.lastExclusiveEndBlock != 0 {
			s.jobsPlanner.SignalCompletionUpUntil(s.name, out.lastExclusiveEndBlock)
		}

		totalDuration := time.Since(start)
		avgDuration := time.Duration(0)
		if out.squashCount > 0 {
			avgDuration = totalDuration / time.Duration(out.squashCount)
		}

		s.log.Info("squashing done", zap.Duration("duration", totalDuration), zap.Duration("squash_avg", avgDuration))
		metrics.SquashProcessesClosed.Inc()
	}
}

type rangeProgress struct {
	squashCount           uint64
	lastExclusiveEndBlock uint64
}

func (s *StoreSquasher) processRanges(ctx context.Context, eg *llerrgroup.Group) (*rangeProgress, error) {
	s.log.Info("processing range", zap.Int("range_count", len(s.ranges)))
	out := &rangeProgress{}
	for {
		if eg.Stop() {
			break
		}

		if len(s.ranges) == 0 {
			s.log.Info("no more ranges to squash")
			return out, nil
		}

		squashableRange := s.ranges[0]
		err := s.processRange(ctx, eg, squashableRange)
		if err == SkipRange {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("process range %s: %w", squashableRange.String(), err)
		}

		// FIXME(abourget): this was `squashCount++` prior.. we're ++ on a EndBlock?!
		out.lastExclusiveEndBlock++

		s.ranges = s.ranges[1:]

		if squashableRange.ExclusiveEndBlock == s.targetExclusiveEndBlock {
			s.targetExclusiveEndBlockReach = true
		}
		s.log.Debug("signaling the jobs planner that we completed", zap.String("module", s.name), zap.Uint64("end_block", squashableRange.ExclusiveEndBlock))
		out.lastExclusiveEndBlock = squashableRange.ExclusiveEndBlock
	}
	return out, nil
}

var SkipRange = errors.New("skip range")

func (s *StoreSquasher) processRange(ctx context.Context, eg *llerrgroup.Group, squashableRange *block.Range) error {
	s.log.Info("testing squashable range",
		zap.Object("range", squashableRange),
		zap.Uint64("next_expected_start_block", s.nextExpectedStartBlock),
	)

	if squashableRange.StartBlock < s.nextExpectedStartBlock {
		return fmt.Errorf("non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.nextExpectedStartBlock, squashableRange.StartBlock, s.ranges)
	}
	if s.nextExpectedStartBlock != squashableRange.StartBlock {
		return SkipRange
	}

	s.log.Debug("found range to merge",
		zap.Stringer("squashable", s),
		zap.Stringer("squashable_range", squashableRange),
	)

	// FIXME(abourget): this is what was prior to the change:
// 	s.log.Debug("found range to merge", zap.Stringer("squashable", s), zap.Stringer("squashable_range", squashableRange))
// 	squashCount++

	nextStore := store.NewPartialKV(s.store.Clone().BaseStore, squashableRange.StartBlock)
	if err := nextStore.Load(ctx, squashableRange.ExclusiveEndBlock); err != nil {
		return fmt.Errorf("initializing next partial store %q: %w", s.name, err)
	}

	s.log.Debug("merging next store loaded", zap.Object("store", nextStore))
	if err := s.store.Merge(nextStore); err != nil {
		return fmt.Errorf("merging: %s", err)
	}

	zlog.Debug("store merge", zap.Object("store", s.store))
	s.nextExpectedStartBlock = squashableRange.ExclusiveEndBlock

	zlog.Info("deleting store", zap.Object("store", nextStore))
	eg.Go(func() error {
		_ = nextStore.DeleteStore(ctx, squashableRange.ExclusiveEndBlock)
		return nil
	})

	if s.shouldSaveFullKV(s.store.InitialBlock(), squashableRange) {
		eg.Go(func() error {
			_, err := s.store.Save(ctx, squashableRange.ExclusiveEndBlock)
			return err
		})
	}
	return nil
}

func (s *StoreSquasher) shouldSaveFullKV(storeInitialBlock uint64, squashableRange *block.Range) bool {
	// TODO(abourget): beautiful opportunity to create a table-driven tests function
	// called `shouldSaveFullKV(...)` .. and wrap the `Save()` call in it.
	// These are crazy conditions that are unreadable.. don't force us to dig into this
	// at this level, if the only question is "should I save or not?"
	// we'll dig in the conditions if we need

	isSaveIntervalReached := squashableRange.ExclusiveEndBlock%s.storeSaveInterval == 0
	isFirstKvForModule := isSaveIntervalReached && squashableRange.StartBlock == storeInitialBlock
	isCompletedKv := isSaveIntervalReached && squashableRange.Len()-s.storeSaveInterval == 0
	// This log was for debugging/testing in prod
	zlog.Info("should write full store?",
		zap.Uint64("exclusiveEndBlock", squashableRange.ExclusiveEndBlock),
		zap.Uint64("store_interval", s.storeSaveInterval),
		zap.Bool("is_save_interval_reached", isSaveIntervalReached),
		zap.Bool("is_first_kv_for_module", isFirstKvForModule),
		zap.Bool("is_completed_kv", isCompletedKv),
	)

	return isFirstKvForModule || isCompletedKv
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
