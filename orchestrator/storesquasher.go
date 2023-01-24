package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/streamingfast/substreams/storage/store"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

var SkipRange = errors.New("skip range")
var PartialChunksDone = errors.New("partial chunks done")

type StoreSquasher struct {
	name                         string
	store                        *store.FullKV
	requestRange                 *block.Range
	ranges                       block.Ranges
	targetExclusiveEndBlock      uint64 // The upper bound of this Squasher's responsibility
	targetExclusiveEndBlockReach bool
	nextExpectedStartBlock       uint64 // This goes from a lower number up to `targetExclusiveEndBlock`
	//log                          *zap.Logger
	partialsChunks      chan block.Ranges
	waitForCompletionCh chan error
	storeSaveInterval   uint64

	onStoreCompletedUntilBlock func(storeName string, blockNum uint64)
}

func NewStoreSquasher(
	initialStore *store.FullKV,
	targetExclusiveBlock,
	nextExpectedStartBlock uint64,
	storeSaveInterval uint64,
	onStoreCompletedUntilBlock func(storeName string, blockNum uint64),
) *StoreSquasher {
	s := &StoreSquasher{
		name:                       initialStore.Name(),
		store:                      initialStore,
		targetExclusiveEndBlock:    targetExclusiveBlock,
		nextExpectedStartBlock:     nextExpectedStartBlock,
		onStoreCompletedUntilBlock: onStoreCompletedUntilBlock,
		storeSaveInterval:          storeSaveInterval,
		partialsChunks:             make(chan block.Ranges, 100 /* before buffering the upstream requests? */),
		waitForCompletionCh:        make(chan error),
	}
	return s
}

func (s *StoreSquasher) moduleName() string { return s.name }

func (s *StoreSquasher) waitForCompletion(ctx context.Context) error {
	logger := s.logger(ctx)

	// TODO(abourget): unsure what this line means, a `close()` doesn't wait?
	logger.Info("waiting form terminate after partials chucks chan empty")
	close(s.partialsChunks)

	logger.Info("waiting for completion")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-s.waitForCompletionCh:
		if err != nil {
			return fmt.Errorf("store squasher waiting for completion: %w", err)
		}
		logger.Info("squasher completed")
		return nil
	}
}

func (s *StoreSquasher) squash(ctx context.Context, partialsChunks block.Ranges) error {
	if len(partialsChunks) == 0 {
		return fmt.Errorf("partialsChunks is empty for module %q", s.name)
	}

	select {
	case s.partialsChunks <- partialsChunks:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (s *StoreSquasher) logger(ctx context.Context) *zap.Logger {
	return reqctx.Logger(ctx).With(zap.String("store_name", s.store.Name()), zap.String("module_hash", s.store.ModuleHash()))
}

func (s *StoreSquasher) launch(ctx context.Context) {
	logger := s.logger(ctx)
	reqStats := reqctx.ReqStats(ctx)

	logger.Info("launching store squasher")
	metrics.SquashersStarted.Inc()
	defer metrics.SquashersEnded.Inc()

	for {
		if err := s.getPartialChunks(ctx); err != nil {
			if errors.Is(err, PartialChunksDone) {
				close(s.waitForCompletionCh)
				return
			}
			s.waitForCompletionCh <- err
			return
		}

		eg := llerrgroup.New(250)
		start := time.Now()

		out, err := s.processRanges(ctx, eg)
		if err != nil {
			s.waitForCompletionCh <- err
			return
		}

		if err := eg.Wait(); err != nil {
			s.waitForCompletionCh <- fmt.Errorf("waiting: %w", err)
			return
		}

		if out.lastExclusiveEndBlock != 0 {
			s.onStoreCompletedUntilBlock(s.name, out.lastExclusiveEndBlock)
			reqStats.RecordStoreSquasherProgress(s.name, out.lastExclusiveEndBlock)
		}

		totalDuration := time.Since(start)
		avgDuration := time.Duration(0)
		if out.squashCount > 0 {
			metrics.SquashesLaunched.AddInt(int(out.squashCount))
			avgDuration = totalDuration / time.Duration(out.squashCount)
		}
		logger.Info("squashing done", zap.Duration("duration", totalDuration), zap.Duration("squash_avg", avgDuration))
	}
}

type rangeProgress struct {
	squashCount           uint64
	lastExclusiveEndBlock uint64
}

func (s *StoreSquasher) sortRange() {
	sort.Slice(s.ranges, func(i, j int) bool {
		return s.ranges[i].StartBlock < s.ranges[j].ExclusiveEndBlock
	})
}

func (s *StoreSquasher) getPartialChunks(ctx context.Context) error {
	logger := s.logger(ctx)

	select {
	case <-ctx.Done():
		logger.Info("quitting on a close context")
		return ctx.Err()

	case partialsChunks, ok := <-s.partialsChunks:
		if !ok {
			logger.Info("squashing done, no more partial chunks to squash")
			return PartialChunksDone
		}
		logger.Info("got partials chunks", zap.Stringer("partials_chunks", partialsChunks))
		s.ranges = append(s.ranges, partialsChunks...)
		s.sortRange()
	}
	return nil
}

// store_save_interval = 1K
// 0 -> 10K

//j2 0 -> 2		pw => 0-1, 1-2
//j3 2 -> 4 	pw => 2-3, 3-4
//j4 4 -> 6
//j5 6 -> 8
//j6 8 -> 10

func (s *StoreSquasher) processRanges(ctx context.Context, eg *llerrgroup.Group) (*rangeProgress, error) {
	logger := s.logger(ctx)
	logger.Info("processing range", zap.Int("range_count", len(s.ranges)))
	out := &rangeProgress{}
	for {
		if eg.Stop() {
			break
		}

		if len(s.ranges) == 0 {
			logger.Info("no more ranges to squash")
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

		out.squashCount++

		s.ranges = s.ranges[1:]

		if squashableRange.ExclusiveEndBlock == s.targetExclusiveEndBlock {
			s.targetExclusiveEndBlockReach = true
		}
		logger.Debug("signaling the jobs planner that we completed", zap.String("module", s.name), zap.Uint64("end_block", squashableRange.ExclusiveEndBlock))
		out.lastExclusiveEndBlock = squashableRange.ExclusiveEndBlock
	}
	return out, nil
}

func (s *StoreSquasher) processRange(ctx context.Context, eg *llerrgroup.Group, squashableRange *block.Range) error {
	logger := s.logger(ctx)

	logger.Info("testing squashable range",
		zap.Object("range", squashableRange),
		zap.Uint64("next_expected_start_block", s.nextExpectedStartBlock),
	)

	if squashableRange.StartBlock < s.nextExpectedStartBlock {
		return fmt.Errorf("non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.nextExpectedStartBlock, squashableRange.StartBlock, s.ranges)
	}
	if s.nextExpectedStartBlock != squashableRange.StartBlock {
		return SkipRange
	}

	logger.Debug("found range to merge",
		zap.Stringer("squashable", s),
		zap.Stringer("squashable_range", squashableRange),
	)

	nextStore := s.store.DerivePartialStore(squashableRange.StartBlock)
	if err := nextStore.Load(ctx, squashableRange.ExclusiveEndBlock); err != nil {
		return fmt.Errorf("initializing next partial store %q: %w", s.name, err)
	}

	logger.Debug("merging next store loaded", zap.Object("store", nextStore))
	if err := s.store.Merge(nextStore); err != nil {
		return fmt.Errorf("merging: %w", err)
	}

	logger.Debug("store merge", zap.Object("store", s.store))
	s.nextExpectedStartBlock = squashableRange.ExclusiveEndBlock

	logger.Info("deleting store", zap.Stringer("store", nextStore))
	eg.Go(func() error {
		return nextStore.DeleteStore(ctx, squashableRange.ExclusiveEndBlock)
	})

	if s.shouldSaveFullKV(s.store.InitialBlock(), squashableRange) {
		_, writer, err := s.store.Save(squashableRange.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("save full store: %w", err)
		}

		eg.Go(func() error {
			return writer.Write(ctx)
		})
	}
	return nil
}

func (s *StoreSquasher) shouldSaveFullKV(storeInitialBlock uint64, squashableRange *block.Range) bool {

	// we check if the squashableRange we just merged into our FullKV store, ends on a storeInterval boundary block
	// If someone the storeSaveInterval

	// squashable range must end on a store boundary block
	isSaveIntervalReached := squashableRange.ExclusiveEndBlock%s.storeSaveInterval == 0
	// we expect the range to be euqal to the store save interval, except if the range start block
	// is the same as the store initial block
	isFirstKvForModule := isSaveIntervalReached && squashableRange.StartBlock == storeInitialBlock
	isCompletedKv := isSaveIntervalReached && squashableRange.Len()-s.storeSaveInterval == 0
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
