package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/streamingfast/shutter"

	"github.com/streamingfast/substreams/storage/store"

	"github.com/abourget/llerrgroup"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
)

var SkipRange = errors.New("skip range")
var PartialsChannelClosed = errors.New("partial chunks done")

type StoreSquasher struct {
	*shutter.Shutter

	name                         string
	store                        *store.FullKV
	requestRange                 *block.Range
	ranges                       block.Ranges
	targetExclusiveEndBlock      uint64 // The upper bound of this Squasher's responsibility
	targetExclusiveEndBlockReach bool
	nextExpectedStartBlock       uint64 // This goes from a lower number up to `targetExclusiveEndBlock`
	//log                          *zap.Logger
	partialsChunks    chan block.Ranges
	storeSaveInterval uint64

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
		Shutter:                    shutter.New(),
		name:                       initialStore.Name(),
		store:                      initialStore,
		targetExclusiveEndBlock:    targetExclusiveBlock,
		nextExpectedStartBlock:     nextExpectedStartBlock,
		onStoreCompletedUntilBlock: onStoreCompletedUntilBlock,
		storeSaveInterval:          storeSaveInterval,
		partialsChunks:             make(chan block.Ranges, 100 /* before buffering the upstream requests? */),
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
	case <-s.Terminated():
		if err := s.Err(); err != nil {
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
	case <-s.Terminated():
		return s.Err()
	}
	return nil
}

func (s *StoreSquasher) logger(ctx context.Context) *zap.Logger {
	return reqctx.Logger(ctx).With(zap.String("store_name", s.store.Name()), zap.String("module_hash", s.store.ModuleHash()))
}

func (s *StoreSquasher) launch(ctx context.Context) {
	err := s.processPartials(ctx)
	s.Shutdown(err)
}
func (s *StoreSquasher) processPartials(ctx context.Context) error {
	logger := s.logger(ctx)
	reqStats := reqctx.ReqStats(ctx)

	logger.Info("launching store squasher")
	metrics.SquashersStarted.Inc()
	defer metrics.SquashersEnded.Inc()

	for {
		if err := s.accumulateMorePartials(ctx); err != nil {
			if errors.Is(err, PartialsChannelClosed) {
				return nil
			}
			return err
		}

		eg := llerrgroup.New(250)
		start := time.Now()

		out, err := s.processRanges(ctx, eg)
		if err != nil {
			return err
		}

		if err := eg.Wait(); err != nil {
			return fmt.Errorf("waiting: %w", err)
		}

		if out.lastExclusiveEndBlock != 0 {
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

func (s *StoreSquasher) ensureNoOverlap() error {
	if len(s.ranges) < 2 {
		return nil
	}
	end := len(s.ranges) - 1
	for i := 0; i < end; i++ {
		left := s.ranges[i]
		right := s.ranges[i+1]
		if right.StartBlock < left.ExclusiveEndBlock {
			return fmt.Errorf("sorted ranges overlapping, left: %s, right: %s", left, right)
		}
	}
	return nil
}

func (s *StoreSquasher) accumulateMorePartials(ctx context.Context) error {
	logger := s.logger(ctx)

	select {
	case <-s.Terminated():
		return s.Err()
	case <-ctx.Done():
		logger.Info("quitting on a close context")
		return ctx.Err()

	case partialsChunks, ok := <-s.partialsChunks:
		if !ok {
			logger.Info("squashing done, no more partial chunks to squash")
			return PartialsChannelClosed
		}
		logger.Info("got partials chunks", zap.Stringer("partials_chunks", partialsChunks))
		s.ranges = append(s.ranges, partialsChunks...)
		s.sortRange()
		if err := s.ensureNoOverlap(); err != nil {
			return err
		}
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
		// This will inform the scheduler that this range has progressed, as it affects jobs dependending on it
		s.onStoreCompletedUntilBlock(s.name, squashableRange.ExclusiveEndBlock)

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

	startTime := time.Now()
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

	loadTime := time.Now()
	if err := nextStore.Load(ctx, squashableRange.ExclusiveEndBlock); err != nil {
		return fmt.Errorf("initializing next partial store %q: %w", s.name, err)
	}
	loadTimeTook := time.Since(loadTime)

	mergeTime := time.Now()
	logger.Info("merging next store loaded", zap.Object("store", nextStore))
	if err := s.store.Merge(nextStore); err != nil {
		return fmt.Errorf("merging: %w", err)
	}
	mergeTimeTook := time.Since(mergeTime)

	logger.Debug("store merge", zap.Object("store", s.store))
	s.nextExpectedStartBlock = squashableRange.ExclusiveEndBlock

	if reqctx.Details(ctx).ProductionMode || squashableRange.ExclusiveEndBlock%s.storeSaveInterval == 0 {
		logger.Info("deleting store", zap.Stringer("store", nextStore))
		eg.Go(func() error {
			return nextStore.DeleteStore(ctx, squashableRange.ExclusiveEndBlock)
		})
	}

	if s.shouldSaveFullKV(s.store.InitialBlock(), squashableRange) {
		saveTime := time.Now()
		_, writer, err := s.store.Save(squashableRange.ExclusiveEndBlock)
		saveTimeTook := time.Since(saveTime)
		if err != nil {
			return fmt.Errorf("save full store: %w", err)
		}

		eg.Go(func() error {
			// TODO: could this cause an issue if the writing takes more time than when trying to opening the file??
			return writer.Write(ctx)
		})

		logger.Info(
			"squashing time metrics",
			zap.String("load_time", loadTimeTook.String()),
			zap.String("merge_time", mergeTimeTook.String()),
			zap.String("save_time", saveTimeTook.String()),
			zap.String("total_time", time.Since(startTime).String()),
		)
	}

	return nil
}

func (s *StoreSquasher) shouldSaveFullKV(storeInitialBlock uint64, squashableRange *block.Range) bool {
	// we check if the squashableRange we just merged into our FullKV store, ends on a storeInterval boundary block
	// If someone the storeSaveInterval

	// squashable range must end on a store boundary block
	isSaveIntervalReached := squashableRange.ExclusiveEndBlock%s.storeSaveInterval == 0
	// we expect the range to be equal to the store save interval, except if the range start block
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
