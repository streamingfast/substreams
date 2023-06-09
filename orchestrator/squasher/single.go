package squasher

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

var SkipFile = errors.New("skip file")
var PartialsChannelClosed = errors.New("partial chunks done")

type SingleState int

const (
	SingleIdle SingleState = iota
	SingleMerging
	SingleCompleted // All merging operations were completed for the provided Segmenter
)

type Single struct {
	*shutter.Shutter

	state SingleState

	name   string
	logger *zap.Logger
	store  *store.FullKV

	segmenter           *block.Segmenter
	nextSegmentToSquash int
	partialsPresent     map[int]bool // by segment index

	writerErrGroup *llerrgroup.Group
}

func NewSingle(
	ctx context.Context,
	initialStore *store.FullKV,
	segmenter *block.Segmenter,
	nextSegmentToSquash int,
) *Single {
	logger := reqctx.Logger(ctx).With(zap.String("store_name", initialStore.Name()), zap.String("module_hash", initialStore.ModuleHash()))
	s := &Single{
		Shutter:             shutter.New(),
		name:                initialStore.Name(),
		logger:              logger,
		store:               initialStore,
		segmenter:           segmenter,
		nextSegmentToSquash: nextSegmentToSquash,
		partialsPresent:     make(map[int]bool),
		writerErrGroup:      llerrgroup.New(250),
	}
	return s
}

func (s *Single) AddPartial(unit stage.Unit) loop.Cmd {
	s.partialsPresent[unit.Segment] = true
	s.sortRange()

	if err := s.ensureNoOverlap(); err != nil {
		return loop.Quit(err)
	}
	return s.CmdMergeRange()
}

func (s *Single) NextRange() {
	switch s.state {
	case SingleMerging:
		s.state = SingleIdle
	case SingleCompleted:
		return
	}
	s.nextSegmentToSquash++
}

func (s *Single) CmdMergeRange() loop.Cmd {
	if s.state != SingleIdle {
		return nil
	}

	nextRange := s.segmenter.Range(s.nextSegmentToSquash)
	if nextRange == nil {
		s.state = SingleCompleted
		return func() loop.Msg {
			return MsgStoreCompleted{}
		}
	}

	var nextFile *store.FileInfo
	for _, file := range s.files {
		if file.Range.Equals(nextRange) {
			nextFile = file
		}
	}

	if nextFile == nil {
		// Nothing contiguous to merge at the moment.
		return nil
	}

	// TODO: check whether we're in a state to actually merge anything
	//  is there some contiguous stuff I can do?
	//  If not, return nil
	s.state = SingleMerging

	return func() loop.Msg {
		// FIXME: transform into a Staged operation, with all the files
		// for a given range would be done in parallel here with a simple `llerrgroup`
		// .. all stores are merged in one swift, from the Squasher's perspective.
		// TODO: Do the actual merging
		return MsgMergeFinished{ModuleName: s.name}
	}
}

func (s *Single) moduleName() string { return s.name }

//func (s *Single) squash(ctx context.Context, partialsChunks store.FileInfos) error {
//	if len(partialsChunks) == 0 {
//		return fmt.Errorf("partialsChunks is empty for module %q", s.name)
//	}
//
//	select {
//	case s.partialsChunks <- partialsChunks:
//	case <-ctx.Done():
//		return ctx.Err()
//	case <-s.Terminated():
//		return s.Err()
//	}
//	return nil
//}
//
//func (s *Single) logger(ctx context.Context) *zap.Logger {
//	return
//}

//func (s *Single) launch(ctx context.Context) {
//	ctx, span := reqctx.WithSpan(ctx, fmt.Sprintf("substreams/tier1/pipeline/store_squasher/%s/squashing", s.name))
//	span.SetAttributes(
//		attribute.Int64("target_exclusive_end_block", int64(s.targetExclusiveEndBlock)),
//		attribute.Int64("next_expected_start_block", int64(s.nextExpectedStartBlock)),
//	)
//	err := s.processPartials(ctx)
//	span.EndWithErr(&err)
//	s.Shutdown(err)
//}

// TODO: this function needs to be turned into a message
func (s *Single) processPartials(ctx context.Context) error {
	reqStats := reqctx.ReqStats(ctx)

	s.logger.Info("launching store squasher")
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
		s.logger.Info("squashing done", zap.Duration("duration", totalDuration), zap.Duration("squash_avg", avgDuration))
	}
}

type rangeProgress struct {
	squashCount           uint64
	lastExclusiveEndBlock uint64
}

func (s *Single) sortRange() {
	sort.Slice(s.files, func(i, j int) bool {
		return s.files[i].Range.StartBlock < s.files[j].Range.ExclusiveEndBlock
	})
}

func (s *Single) ensureNoOverlap() error {
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

// store_save_interval = 1K
// 0 -> 10K

//j2 0 -> 2		pw => 0-1, 1-2
//j3 2 -> 4 	pw => 2-3, 3-4
//j4 4 -> 6
//j5 6 -> 8
//j6 8 -> 10

func (s *Single) processRanges(ctx context.Context, eg *llerrgroup.Group) (*rangeProgress, error) {
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

func (s *Single) processSquashableFile(ctx context.Context, eg *llerrgroup.Group, squashableFile *store.FileInfo) error {
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

func (s *Single) shouldSaveFullKV(storeInitialBlock uint64, squashableRange *block.Range) bool {
	// we check if the squashableRange we just merged into our FullKV store, ends on a storeInterval boundary block
	// If someone the storeSaveInterval

	// squasher range must end on a store boundary block
	isSaveIntervalReached := squashableRange.ExclusiveEndBlock%s.storeSaveInterval == 0
	// we expect the range to be equal to the store save interval, except if the range start block
	// is the same as the store initial block
	isFirstKvForModule := isSaveIntervalReached && squashableRange.StartBlock == storeInitialBlock
	isCompletedKv := isSaveIntervalReached && squashableRange.Len()-s.storeSaveInterval == 0
	return isFirstKvForModule || isCompletedKv
}

func (s *Single) IsEmpty() bool {
	return len(s.files) == 0
}

func (s *Single) String() string {
	var add string
	if s.targetExclusiveEndBlockReach {
		add = " (target reached)"
	}
	return fmt.Sprintf("%s%s: [%s]", s.name, add, s.files)
}
