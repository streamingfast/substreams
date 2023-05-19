package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"sort"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"go.uber.org/zap"
)

var SkipFile = errors.New("skip file")
var PartialsChannelClosed = errors.New("partial chunks done")

type StoreSquasher struct {
	*shutter.Shutter

	name                         string
	store                        *store.FullKV
	files                        store.FileInfos
	targetExclusiveEndBlock      uint64 // The upper bound of this Squasher's responsibility
	targetExclusiveEndBlockReach bool
	nextExpectedStartBlock       uint64 // This goes from a lower number up to `targetExclusiveEndBlock`
	partialsChunks               chan store.FileInfos
	storeSaveInterval            uint64

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
		partialsChunks:             make(chan store.FileInfos, 100 /* before buffering the upstream requests? */),
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

func (s *StoreSquasher) squash(ctx context.Context, partialsChunks store.FileInfos) error {
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
	ctx, span := reqctx.WithSpan(ctx, fmt.Sprintf("substreams/tier1/pipeline/store_squasher/%s/squashing", s.name))
	span.SetAttributes(
		attribute.Int64("target_exclusive_end_block", int64(s.targetExclusiveEndBlock)),
		attribute.Int64("next_expected_start_block", int64(s.nextExpectedStartBlock)),
	)
	err := s.processPartials(ctx)
	span.EndWithErr(&err)
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
	sort.Slice(s.files, func(i, j int) bool {
		return s.files[i].Range.StartBlock < s.files[j].Range.ExclusiveEndBlock
	})
}

func (s *StoreSquasher) ensureNoOverlap() error {
	if len(s.files) < 2 {
		return nil
	}

	end := len(s.files) - 1
	for i := 0; i < end; i++ {
		left := s.files[i]
		right := s.files[i+1]

		if right.Range.StartBlock < left.Range.ExclusiveEndBlock {
			return fmt.Errorf("sorted ranges overlapping, left: %s, right: %s", left.Range, right.Range)
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
		s.files = append(s.files, partialsChunks...)
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
	logger.Info("processing range", zap.Int("range_count", len(s.files)))
	out := &rangeProgress{}
	for {
		if eg.Stop() {
			break
		}

		if len(s.files) == 0 {
			logger.Info("no more ranges to squash")
			return out, nil
		}

		squashableFile := s.files[0]
		err := s.processSquashableFile(ctx, eg, squashableFile)
		if err == SkipFile {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("process squashable file on range %q: %w", squashableFile.Range.String(), err)
		}
		// This will inform the scheduler that this range has progressed, as it affects jobs dependending on it
		s.onStoreCompletedUntilBlock(s.name, squashableFile.Range.ExclusiveEndBlock)

		out.squashCount++

		s.files = s.files[1:]

		if squashableFile.Range.ExclusiveEndBlock == s.targetExclusiveEndBlock {
			s.targetExclusiveEndBlockReach = true
		}
		logger.Debug("signaling the jobs planner that we completed", zap.String("module", s.name), zap.String("file", squashableFile.Filename))
		out.lastExclusiveEndBlock = squashableFile.Range.ExclusiveEndBlock
	}
	return out, nil
}

func (s *StoreSquasher) processSquashableFile(ctx context.Context, eg *llerrgroup.Group, squashableFile *store.FileInfo) error {
	logger := s.logger(ctx)

	startTime := time.Now()
	logger.Info("testing squashable range",
		zap.Stringer("range", squashableFile.Range),
		zap.Uint64("next_expected_start_block", s.nextExpectedStartBlock),
	)

	if squashableFile.Range.StartBlock < s.nextExpectedStartBlock {
		return fmt.Errorf("non contiguous ranges were added to the store squasher, expected %d, got %d, ranges: %s", s.nextExpectedStartBlock, squashableFile.Range.StartBlock, s.files)
	}
	if s.nextExpectedStartBlock != squashableFile.Range.StartBlock {
		return SkipFile
	}

	logger.Debug("found range to merge",
		zap.Stringer("squasher", s),
		zap.String("squashable_file", squashableFile.Filename),
	)

	nextStore := s.store.DerivePartialStore(squashableFile.Range.StartBlock)

	loadTime := time.Now()
	if err := nextStore.Load(ctx, squashableFile); err != nil {
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
	s.nextExpectedStartBlock = squashableFile.Range.ExclusiveEndBlock

	if reqctx.Details(ctx).ProductionMode || squashableFile.Range.ExclusiveEndBlock%s.storeSaveInterval == 0 {
		logger.Info("deleting store", zap.Stringer("store", nextStore))
		eg.Go(func() error {
			return nextStore.DeleteStore(ctx, squashableFile)
		})
	}

	if s.shouldSaveFullKV(s.store.InitialBlock(), squashableFile.Range) {
		saveTime := time.Now()
		_, writer, err := s.store.Save(squashableFile.Range.ExclusiveEndBlock)
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
	return len(s.files) == 0
}

func (s *StoreSquasher) String() string {
	var add string
	if s.targetExclusiveEndBlockReach {
		add = " (target reached)"
	}
	return fmt.Sprintf("%s%s: [%s]", s.name, add, s.files)
}
