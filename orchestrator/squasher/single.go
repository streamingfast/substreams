package squasher

import (
	"context"
	"errors"
	"fmt"
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

// Single represents a single stage's stores squasher.
type Single struct {
	*shutter.Shutter
	ctx context.Context

	state SingleState
	stage int

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
) *Single {
	logger := reqctx.Logger(ctx).With(zap.String("store_name", initialStore.Name()), zap.String("module_hash", initialStore.ModuleHash()))
	s := &Single{
		ctx:                 ctx,
		Shutter:             shutter.New(),
		name:                initialStore.Name(),
		logger:              logger,
		store:               initialStore,
		segmenter:           segmenter,
		nextSegmentToSquash: segmenter.FirstIndex(),
		partialsPresent:     make(map[int]bool),
		writerErrGroup:      llerrgroup.New(250),
	}
	return s
}

func (s *Single) AddPartial(unit stage.Unit) loop.Cmd {
	s.partialsPresent[unit.Segment] = true
	// don't bring me stuff that doesn't align with the Unit,
	// so no need for sorting, no need for overlap checking.s

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
			return stage.MsgasdfasdfStoresCompleted{}
		}
	}

	nextSegmentReady := s.partialsPresent[s.nextSegmentToSquash]
	if !nextSegmentReady {
		return nil
	}

	s.state = SingleMerging

	return func() loop.Msg {
		return s.process(nextRange, s.nextSegmentToSquash)
	}
}

func (s *Single) moduleName() string { return s.name }

// TODO(abourget): Move to the right place:
// 	metrics.SquashersStarted.Inc()
//	defer metrics.SquashersEnded.Inc()

func (s *Single) process(rng *block.Range, segment int) loop.Msg {
	start := time.Now()

	// FIXME: transform into a Staged operation, with all the files
	// for a given range would be done in parallel here with a simple `llerrgroup`
	// .. all stores are merged in one swift, from the Squasher's perspective.
	// TODO: Do the actual merging, of all stores in the stage, in parallel
	//  with an llerrgroup in here.
	//  interrupt on `ctx.Done()`, and early exit when one of the store merge
	//  operation fails.

	// TODO do merging of all stores in parallel here, with an llerrgroup
	out, err := s.processRanges()
	if err != nil {
		return loop.Quit(err)()
	}

	if err := s.writerErrGroup.Wait(); err != nil {
		return fmt.Errorf("waiting: %w", err)
	}

	if out.lastExclusiveEndBlock != 0 {
		// TODO: What's that clause?
	}

	totalDuration := time.Since(start)
	avgDuration := time.Duration(0)
	if out.squashCount > 0 {
		metrics.SquashesLaunched.AddInt(int(out.squashCount))
		avgDuration = totalDuration / time.Duration(out.squashCount)
	}
	s.logger.Info("squashing done", zap.Duration("duration", totalDuration), zap.Duration("squash_avg", avgDuration))

	return stage.MsgMergeFinished{ModuleName: s.name}
}

type rangeProgress struct {
	squashCount           uint64
	lastExclusiveEndBlock uint64
}

// store_save_interval = 1K
// 0 -> 10K

//j2 0 -> 2		pw => 0-1, 1-2
//j3 2 -> 4 	pw => 2-3, 3-4
//j4 4 -> 6
//j5 6 -> 8
//j6 8 -> 10

func (s *Single) processRanges() (*rangeProgress, error) {
	logger := s.logger
	logger.Info("processing range", zap.Int("range_count", len(s.partialsPresent)))
	out := &rangeProgress{}
	for {
		if s.writerErrGroup.Stop() {
			break
		}

		if len(s.partialsPresent) == 0 {
			logger.Info("no more ranges to squash")
			return out, nil
		}

		squashableFile := s.partialsPresent[0]
		err := s.processSquashableFile(squashableFile)
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

func (s *Single) processSquashableFile(squashableFile *store.FileInfo) error {
	logger := s.logger

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
		s.writerErrGroup.Go(func() error {
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

		s.writerErrGroup.Go(func() error {
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
