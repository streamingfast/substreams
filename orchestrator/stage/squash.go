package stage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/go-multierror"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/squash"
	"github.com/streamingfast/substreams/storage/store"
)

// TODO: unify Complete (deprecate word) and FullKV, partial files and PartialKV

// multiSquash is called only when we know that the given mergeUnit has a PartialKV present
// and we know that there is a FullKV store that exists on the previous segment.
// We can therefore, for each module: either use the `store` we have in cache (which was
// perhaps used to _produce_ that prior FullKV), or load it from storage.
// This allows for both initialization of the store, and skipping of FullKV if we
// happen to have some that were deleted.
func (s *Stages) multiSquash(stage *Stage, mergeUnit Unit) error {
	if stage.kind != KindStore {
		panic("multiSquash called on non-store stage")
	}

	// Launch parallel jobs to merge all stages' stores.
	for _, modState := range stage.storeModuleStates {
		if mergeUnit.Segment < modState.segmenter.FirstIndex() {
			continue
		}

		stage.syncWork.Go(func() error {
			stats := reqctx.ReqStats(s.ctx)
			stats.RecordModuleMerging(modState.name)
			defer stats.RecordModuleMergeComplete(modState.name)
			err := s.singleSquash(stage, modState, mergeUnit)
			if err != nil {
				return fmt.Errorf("squash stage %d module %q: %w", stage.idx, modState.name, err)
			}
			return nil
		})
	}

	return stage.syncWork.Wait() // ensure we don't merge the same fullKV multiple times concurrently before it is completely written
}

// A squash run covers at most maxSquashRunSegments segments and stops taking new ones
// once squashRunTimeBudget is spent, so that a long run still reports progress regularly.
const (
	maxSquashRunSegments = 1000
	squashRunTimeBudget  = 30 * time.Second

	// snapshotCopyConcurrency bounds the full store copies in flight for one store module.
	snapshotCopyConcurrency = 32
)

// copyOverEmptyPartials enables copying the previous full store over segments whose
// partials are all empty, instead of merging and saving them one at a time. Tests turn it
// off to compare both paths.
var copyOverEmptyPartials = true

// claimMergeRun marks as Merging the units that directly follow first on its stage and
// already have their partial, up to limit units in total, and returns them all, first
// included. first must already be Merging. The whole run is squashed by a single
// command: squashed one command at a time, each segment would wait for its own round
// trip through the scheduler loop, which is busy with job scheduling.
func (s *Stages) claimMergeRun(stage *Stage, first Unit, limit int) []Unit {
	run := []Unit{first}
	for len(run) < limit {
		next := Unit{Stage: first.Stage, Segment: run[len(run)-1].Segment + 1}
		if next.Segment > stage.segmenter.LastIndex() || s.getState(next) != UnitPartialPresent {
			break
		}
		s.transition(next, UnitMerging, UnitPartialPresent)
		run = append(run, next)
	}
	return run
}

// squashRun squashes steps, consecutive groups of units, in order. It always squashes the
// first step, then stops once budget is spent and returns the units of the steps it did
// not get to as unmerged. On error, merged holds the units of the steps squashed before
// the failing one.
func squashRun(steps [][]Unit, budget time.Duration, squash func(step []Unit) error) (merged, unmerged []Unit, err error) {
	start := time.Now()
	for i, step := range steps {
		if i > 0 && time.Since(start) >= budget {
			return merged, slices.Concat(steps[i:]...), nil
		}
		if err := squash(step); err != nil {
			return merged, nil, err
		}
		merged = append(merged, step...)
	}
	return merged, nil, nil
}

// planSquashSteps splits run into the steps squashRun goes through: each sequence of
// consecutive empty units is one step, and every other unit is a step of its own.
func planSquashSteps(run []Unit, empty map[int]bool) [][]Unit {
	var steps [][]Unit
	for i := 0; i < len(run); {
		j := i + 1
		if empty[run[i].Segment] {
			for j < len(run) && empty[run[j].Segment] {
				j++
			}
		}
		steps = append(steps, run[i:j])
		i = j
	}
	return steps
}

// findEmptyUnits returns the segments of run on which the partial of every store module
// of stage that already started is empty. A module's first segment is never reported:
// there is no earlier full store to copy. Any failure only means fewer segments are
// reported, since the regular squash handles every unit.
func (s *Stages) findEmptyUnits(stage *Stage, run []Unit) map[int]bool {
	if !copyOverEmptyPartials || len(run) < 2 {
		return nil
	}

	emptyFiles := make([]map[string]bool, len(stage.storeModuleStates))
	for i, modState := range stage.storeModuleStates {
		if modState.storeConfig == nil {
			return nil
		}
		var files []*store.FileInfo
		for _, u := range run {
			if u.Segment <= modState.segmenter.FirstIndex() {
				continue
			}
			if rng := modState.segmenter.Range(u.Segment); rng != nil {
				files = append(files, store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock))
			}
		}
		empties, err := modState.storeConfig.EmptyPartialKVs(s.ctx, files)
		if err != nil {
			s.logger.Warn("cannot tell which partials are empty, squashing them all", zap.String("store", modState.name), zap.Error(err))
			return nil
		}
		emptyFiles[i] = empties
	}

	out := make(map[int]bool)
	for _, u := range run {
		empty := true
		for i, modState := range stage.storeModuleStates {
			if u.Segment < modState.segmenter.FirstIndex() {
				continue // not started yet: nothing to squash for it
			}
			rng := modState.segmenter.Range(u.Segment)
			if u.Segment == modState.segmenter.FirstIndex() || rng == nil ||
				!emptyFiles[i][store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock).Filename] {
				empty = false
				break
			}
		}
		if empty {
			out[u.Segment] = true
		}
	}
	return out
}

// squashEmptyUnits squashes consecutive units whose partials are all empty. Such a unit
// leaves every store unchanged, so each of its full stores is a copy of the previous one.
// Copying needs that previous full store both in memory and in storage: until it is, units
// go through the regular squash one at a time. If a copy fails, the units go through the
// regular squash too, which rewrites any full store already copied with the same content.
func (s *Stages) squashEmptyUnits(stage *Stage, units []Unit) error {
	for len(units) > 0 {
		if canCopySnapshots(stage, units[0]) {
			err := s.copySnapshots(stage, units)
			if err == nil {
				return nil
			}
			if ctxErr := s.ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			s.logger.Warn("copying full stores over empty partials failed, squashing them one at a time", zap.Error(err))
			for _, u := range units {
				if err := s.multiSquash(stage, u); err != nil {
					return err
				}
			}
			return nil
		}
		if err := s.multiSquash(stage, units[0]); err != nil {
			return err
		}
		units = units[1:]
		if err := s.ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

// canCopySnapshots reports whether every started store module of stage holds, in memory
// and in storage, the full store as of the start of u.
func canCopySnapshots(stage *Stage, u Unit) bool {
	for _, modState := range stage.storeModuleStates {
		if u.Segment < modState.segmenter.FirstIndex() {
			continue
		}
		rng := modState.segmenter.Range(u.Segment)
		if rng == nil ||
			modState.cachedStore == nil ||
			modState.storeConfig.ModuleInitialBlock() >= rng.StartBlock ||
			modState.lastBlockInStore != rng.StartBlock ||
			modState.snapshotEnd != rng.StartBlock {
			return false
		}
	}
	return true
}

// copySnapshots squashes units, consecutive units whose partials are all empty, by
// copying each started module's current full store to every one of them. The modules only
// move forward once every copy succeeded, so a failure leaves them as they were.
func (s *Stages) copySnapshots(stage *Stage, units []Unit) error {
	var modStates []*StoreModuleState
	for _, modState := range stage.storeModuleStates {
		// A module not started at units[0] does not start within units either: its first
		// segment is never empty, so it never is part of the copied units.
		if units[0].Segment >= modState.segmenter.FirstIndex() {
			modStates = append(modStates, modState)
		}
	}

	lastCopied := make([]uint64, len(modStates))
	var eg errgroup.Group
	for i, modState := range modStates {
		eg.Go(func() error {
			end, err := s.copyModuleSnapshots(modState, units)
			if err != nil {
				return fmt.Errorf("squash stage %d module %q: %w", stage.idx, modState.name, err)
			}
			lastCopied[i] = end
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	for i, modState := range modStates {
		modState.lastBlockInStore = modState.segmenter.Range(units[len(units)-1].Segment).ExclusiveEndBlock
		modState.snapshotEnd = lastCopied[i]

		// Like the regular squash, only delete the partials that a full store now covers.
		partials := make([]*store.FileInfo, 0, len(units))
		for _, u := range units {
			if !modState.segmenter.EndsOnInterval(u.Segment) {
				continue
			}
			rng := modState.segmenter.Range(u.Segment)
			partials = append(partials, store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock))
		}
		go modState.storeConfig.DeletePartialKVs(context.Background(), partials)
	}
	return nil
}

// copyModuleSnapshots copies modState's current full store to each of units that ends on
// the store interval, and returns the end block of the last full store written.
func (s *Stages) copyModuleSnapshots(modState *StoreModuleState, units []Unit) (uint64, error) {
	start := time.Now()
	from := modState.snapshotEnd

	eg, ctx := errgroup.WithContext(s.ctx)
	eg.SetLimit(snapshotCopyConcurrency)
	copies := 0
	lastCopied := from
	for _, u := range units {
		if !modState.segmenter.EndsOnInterval(u.Segment) {
			continue // the regular squash does not save these either
		}
		to := modState.segmenter.Range(u.Segment).ExclusiveEndBlock
		copies++
		lastCopied = max(lastCopied, to)
		eg.Go(func() error {
			return modState.storeConfig.CopyFullKV(ctx, from, to)
		})
	}
	if err := eg.Wait(); err != nil {
		return 0, fmt.Errorf("copying full store at block %d: %w", from, err)
	}

	s.logger.Info("copied full store over empty partials",
		zap.String("store", modState.name),
		zap.Uint64("from_block", from),
		zap.Uint64("up_to_block", lastCopied),
		zap.Int("copies", copies),
		zap.Duration("duration", time.Since(start)),
	)
	return lastCopied, nil
}

type Result struct {
	partialKVStore *store.PartialKV
	partialFile    *store.FileInfo
	fullKVStore    *store.FullKV
	error          error
}

func getPartialOrFullKV(ctx context.Context, modState *StoreModuleState, rng *block.Range) (*store.PartialKV, *store.FileInfo, *store.FullKV, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan Result, 2)
	go func() {
		partialFile := store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock)
		partial := modState.derivePartialKV(rng.StartBlock)
		err := partial.Load(ctx, partialFile)
		results <- Result{partialKVStore: partial, partialFile: partialFile, error: err}
	}()

	go func() {
		nextFull, err := modState.tryLoadFullKV(ctx, rng.ExclusiveEndBlock)
		results <- Result{fullKVStore: nextFull, error: err}
	}()

	var err error
	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			err = multierror.Append(err, ctx.Err())
			return nil, nil, nil, err

		case result := <-results:
			if result.error != nil {
				err = multierror.Append(err, result.error)
				break // from select
			}
			if result.fullKVStore != nil {
				return nil, nil, result.fullKVStore, nil
			}
			if result.partialKVStore != nil {
				return result.partialKVStore, result.partialFile, nil, nil
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("getting partial or full kv: %w", err)
}

// squashStage squashes run. A remote squasher, when configured, gets the whole run
// per module in one RPC so it can keep the full store hot and copy over empty
// partials; tier1 does not read store files. If that remote is unreachable, the run
// is squashed locally instead.
func (s *Stages) squashStage(stage *Stage, run []Unit) (merged, unmerged []Unit, err error) {
	if reqctx.GetRemoteSquasher(s.ctx) != nil {
		err := s.remoteSquashRun(stage, run)
		if err == nil {
			return run, nil, nil
		}
		if !shouldFallbackToLocal(s.ctx, err) {
			return nil, nil, err
		}
		s.logger.Warn("remote squasher not responding, falling back to local squash",
			zap.Int("stage", stage.idx),
			zap.Int("units", len(run)),
			zap.Error(err),
		)
	}
	return s.localSquashRun(stage, run)
}

func (s *Stages) localSquashRun(stage *Stage, run []Unit) (merged, unmerged []Unit, err error) {
	empty := s.findEmptyUnits(stage, run)
	return squashRun(planSquashSteps(run, empty), squashRunTimeBudget, func(step []Unit) error {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		if empty[step[0].Segment] {
			return s.squashEmptyUnits(stage, step)
		}
		return s.multiSquash(stage, step[0])
	})
}

func (s *Stages) remoteSquashRun(stage *Stage, run []Unit) error {
	for _, modState := range stage.storeModuleStates {
		ranges := rangesForModule(modState, run)
		if len(ranges) == 0 {
			continue
		}
		stage.syncWork.Go(func() error {
			if stats := reqctx.ReqStatsOrNil(s.ctx); stats != nil {
				stats.RecordModuleMerging(modState.name)
				defer stats.RecordModuleMergeComplete(modState.name)
			}
			if err := s.remoteSquash(modState, ranges); err != nil {
				return fmt.Errorf("squash stage %d module %q: %w", stage.idx, modState.name, err)
			}
			return nil
		})
	}
	return stage.syncWork.Wait()
}

func rangesForModule(modState *StoreModuleState, run []Unit) []squash.Range {
	var out []squash.Range
	for _, u := range run {
		if u.Segment < modState.segmenter.FirstIndex() {
			continue
		}
		rng := modState.segmenter.Range(u.Segment)
		if rng == nil {
			continue
		}
		out = append(out, squash.Range{StartBlock: rng.StartBlock, ExclusiveEndBlock: rng.ExclusiveEndBlock})
	}
	return out
}

// singleSquash gets the current fullKV and merges the next partialKV into it.
// If there is an existing fullKV at the destination (next segment), it will be loaded instead (whichever file is seen first)
func (s *Stages) singleSquash(stage *Stage, modState *StoreModuleState, mergeUnit Unit) error {
	rng := modState.segmenter.Range(mergeUnit.Segment)
	meter := mergeMetrics{}
	meter.start = time.Now()
	meter.stage = stage.idx
	meter.moduleName = modState.name
	meter.moduleHash = modState.storeConfig.ModuleHash()
	meter.blockRange = rng
	segmentEndsOnInterval := modState.segmenter.EndsOnInterval(mergeUnit.Segment)

	// Retrieve store to merge, from cache or load from storage. Allows skipping of segments
	// for handling partials interspearsed with full KVs.
	meter.getStoreStart = time.Now()
	fullKV, err := modState.getStore(s.ctx, rng.StartBlock) // loads+caches or uses cached store
	if err != nil {
		return fmt.Errorf("getting store: %w", err)
	}
	meter.getStoreEnd = time.Now()

	// Load
	meter.loadStart = time.Now()
	partialKV, partialFile, newFullKV, err := getPartialOrFullKV(s.ctx, modState, rng)
	if err != nil {
		return err
	}
	meter.loadEnd = time.Now()
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}

	if newFullKV != nil {
		if fullKV != nil {
			fullKV.Close()
		}
		modState.cachedStore = newFullKV
		modState.lastBlockInStore = rng.ExclusiveEndBlock
		modState.snapshotEnd = rng.ExclusiveEndBlock
		s.logger.Debug("squashing time metrics (skipped, loaded from full kv)", meter.logFields()...)
		return nil
	}

	metrics.Tier1SquashersStarted.Inc()
	defer metrics.Tier1SquashersEnded.Inc()

	// Merge
	meter.mergeStart = time.Now()
	if err := fullKV.Merge(partialKV); err != nil {
		return fmt.Errorf("merging: %w", err)
	}

	modState.lastBlockInStore = rng.ExclusiveEndBlock
	meter.mergeEnd = time.Now()

	s.logger.Info("merged partial into full store",
		zap.String("store", modState.name),
		zap.Uint64("up_to_block", rng.ExclusiveEndBlock),
		zap.String("store_size", humanize.IBytes(fullKV.SizeBytes())),
	)

	s.logger.Info("deleting partial store", zap.Stringer("store", partialKV))

	// Flush full store
	if segmentEndsOnInterval {
		err := derr.RetryContext(s.ctx, 5, func(ctx context.Context) error {
			meter.saveStart = time.Now()
			_, writer, err := fullKV.Save(rng.ExclusiveEndBlock)
			if err != nil {
				return fmt.Errorf("save full store: %w", err)
			}
			meter.saveEnd = time.Now()
			meter.writeStart = time.Now()
			err = writer.Write(ctx)
			meter.writeEnd = time.Now()
			if err == nil {
				go partialKV.DeleteStore(context.Background(), partialFile)
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("flushing full store: %w", err)
		}
		modState.snapshotEnd = rng.ExclusiveEndBlock
	}

	s.logger.Info("squashing time metrics", meter.logFields()...)

	return nil
}

func shouldFallbackToLocal(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	return isRemoteUnreachable(err)
}

func isRemoteUnreachable(err error) bool {
	if grpcErr := dgrpc.AsGRPCError(err); grpcErr != nil {
		switch grpcErr.Code() {
		case codes.Unavailable, codes.DeadlineExceeded:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func (s *Stages) remoteSquash(modState *StoreModuleState, ranges []squash.Range) error {
	remote := reqctx.GetRemoteSquasher(s.ctx)
	req := squash.Request{
		ModuleName:           modState.name,
		ModuleHash:           modState.storeConfig.ModuleHash(),
		ModuleInitialBlock:   modState.storeConfig.ModuleInitialBlock(),
		UpdatePolicy:         modState.storeConfig.UpdatePolicy(),
		ValueType:            modState.storeConfig.ValueType(),
		StateStore:           remote.StateStoreURL,
		StateStoreDefaultTag: remote.CacheTag,
		Ranges:               ranges,
		StoreSizeLimit:       remote.StoreSizeLimit,
	}

	var resp *squash.Result
	err := derr.RetryContext(s.ctx, 5, func(ctx context.Context) error {
		var callErr error
		resp, callErr = remote.Client.Squash(ctx, req)
		if callErr == nil {
			return nil
		}
		if grpcErr := dgrpc.AsGRPCError(callErr); grpcErr != nil {
			switch grpcErr.Code() {
			case codes.InvalidArgument, codes.FailedPrecondition, codes.NotFound:
				return derr.NewFatalError(callErr)
			}
		}
		return callErr
	})
	first, last := ranges[0].StartBlock, ranges[len(ranges)-1].ExclusiveEndBlock
	if err != nil {
		return fmt.Errorf("remote squash %s [%d-%d]: %w", modState.name, first, last, err)
	}

	s.logger.Info("remote squash completed",
		zap.String("store", modState.name),
		zap.Int("segments", len(ranges)),
		zap.Uint64("start_block", first),
		zap.Uint64("up_to_block", last),
		zap.String("store_size", humanize.IBytes(resp.StoreSizeBytes)),
		zap.Bool("loaded_existing_full_kv", resp.LoadedExisting),
	)
	return nil
}
