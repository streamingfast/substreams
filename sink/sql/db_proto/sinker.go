package db_proto

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/logging/zapx"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	sink "github.com/streamingfast/substreams/sink"
	sql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/stats"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Sinker struct {
	*sink.Sinker
	db                    sql.Database
	useTransaction        bool
	blockBatchSize        uint64
	stats                 *stats.Stats
	logger                *zap.Logger
	rootMessageDescriptor protoreflect.MessageDescriptor
	lastAppliedBlockNum   uint64
	lastAppliedBlockTime  time.Time

	// directInserts records that the stream reached the chain head and the database was
	// switched off any buffered write path. It only ever goes from false to true.
	directInserts bool

	// constraints decides what is created and when. Anything not applied upfront is
	// applied once the backfill is over, which is the point of the default.
	constraints        sql.ConstraintPolicy
	constraintsApplied bool

	// holding buffers the blocks received since the last flush. It is only ever touched
	// from the sinker's callbacks, which are called sequentially.
	holding []*Holder

	decoder *decoder
}

// NewSinker builds the from-proto sinker. decodeWorkers bounds how many blocks are
// unmarshalled and walked concurrently at flush time; zero picks one per core, less one,
// capped at eight.
func NewSinker(rootMessageDescriptor protoreflect.MessageDescriptor, sink *sink.Sinker, db sql.Database, useTransaction bool, constraints sql.ConstraintPolicy, blockBatchSize int, decodeWorkers int, stats *stats.Stats, logger *zap.Logger) *Sinker {
	return &Sinker{
		db:                    db,
		rootMessageDescriptor: rootMessageDescriptor,
		useTransaction:        useTransaction,
		constraints:           constraints,
		blockBatchSize:        uint64(blockBatchSize),
		stats:                 stats,
		Sinker:                sink,
		logger:                logger,
		decoder:               newDecoder(rootMessageDescriptor, decodeWorkers),
	}
}

func (s *Sinker) Run(ctx context.Context) error {
	// Show stats one last time before exiting run
	defer s.LogStats()
	defer s.closeDatabase()

	cursor, err := s.db.FetchCursor()
	if err != nil {
		return fmt.Errorf("fetch cursor: %w", err)
	}

	// The step of the stored cursor deliberately decides nothing here. It says where the
	// previous run stopped, not where the chain is now: a sink that was live, went down for
	// an hour and comes back has a STEP_NEW — or, if its block forked out meanwhile, a
	// STEP_UNDO — cursor with a full backfill ahead of it, which is precisely the run the
	// spool exists for. Only the step of the blocks now arriving says whether we are at the
	// head, and HandleBlockScopedData already reads it per block.
	//
	// A run that resumes on a forked-out block gets the undo signal as its first message,
	// before any block. That is handled where it lands: HandleBlockUndoSignal drains the
	// spool, which is empty, deletes the rows and records the cursor on the open segment.
	// Whatever follows — irreversible blocks to catch up on, or live ones — then decides
	// what happens to the spool, as it would on any other run.
	//clean up the mess from running without a transaction
	if cursor != nil {
		err = s.db.HandleBlocksUndo(cursor.Block().Num())
		if err != nil {
			return fmt.Errorf("handle blocks undo from %s: %w", cursor.Block(), err)
		}
	}
	//panic("Testing 12 12")
	s.logger.Info("fetched cursor", zap.Stringer("block", cursor.Block()))
	if cursor != nil {
		// Seed the applied mark, otherwise the first downloaded block would be measured
		// against zero and reported as a chain-height-sized backlog.
		s.stats.Progress.SetResumeBlock(cursor.Block().Num())
	}

	s.stats.LastBlockProcessAt = time.Now()
	s.Sinker.Run(ctx, cursor, s)

	return s.Sinker.Err()
}

func (s *Sinker) LogStats() {
	s.stats.Log()
}

// closeTimeout bounds the shutdown seal. Writing the open segment's manifest and fsyncing
// it is local work of a few milliseconds; anything past this is a disk that is not coming
// back, and holding the process there helps nobody.
const closeTimeout = 30 * time.Second

// closeDatabase seals whatever is still held and releases the connections, on every way
// out of the run rather than only on the one that ends a bounded range.
//
// A backfill is normally interrupted rather than completed — Ctrl-C, a killed pod, a
// stream that errors — and not sealing the open segment on the way out throws away every
// block in it, up to --db-write-max-size: recovery discards an unsealed segment, so those
// blocks are streamed, and paid for, a second time. That is the outcome the spool exists
// to prevent.
//
// It closes on a fresh context because the run's is already cancelled by the time an
// interrupt reaches here, and the seal it has to perform is a write.
func (s *Sinker) closeDatabase() {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	if err := s.db.Close(ctx); err != nil {
		s.logger.Warn("closing the database", zap.Error(err))
	}
}

type Holder struct {
	output *pbsubstreamsrpc.MapModuleOutput
	data   *pbsubstreamsrpc.BlockScopedData
	isLive *bool
	cursor *sink.Cursor
}

func (s *Sinker) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) (err error) {
	output := data.Output

	if output.Name == "" {
		return nil
	}

	if output.Name != s.OutputModuleName() {
		return fmt.Errorf("received data from wrong output module, expected to received from %q but got module's output for %q", s.OutputModuleName(), output.Name)
	}

	startAt := time.Now()
	defer func() {
		s.stats.LastBlockProcessAt = time.Now()
		s.stats.BlockProcessingDuration.Add(time.Since(startAt))
		s.stats.TotalProcessingDuration += time.Since(startAt)
	}()

	if s.stats.BlockCount > 0 {
		s.stats.WaitDurationBetweenBlocks.Add(time.Since(s.stats.LastBlockProcessAt))
		s.stats.TotalDurationBetween += time.Since(s.stats.LastBlockProcessAt)
	}
	s.stats.BlockCount++

	// The switch happens before this block is held, which is what makes the spool safe
	// against reorgs: `isLive` here comes from the cursor-based liveness checker the sink
	// installs itself (see runFromProtoSink; --live-block-time-delta is deliberately not
	// registered on this command), so it turns true on the first cursor at STEP_NEW —
	// the first block that can ever be undone. Everything spooled up to here was delivered
	// at STEP_NEW_IRREVERSIBLE and cannot be undone, and from here on there is no spool.
	// An undo therefore never reaches a spool holding undoable blocks. It can still reach a
	// spool holding nothing — a run resuming on a block that forked out while it was down
	// gets the undo before any block — which is what the drain in HandleBlocksUndo is for.
	if isLive != nil && *isLive && !s.directInserts {
		// Write what is held before the switch, with the cursor of the last held block:
		// those blocks belong to the buffered path, and this block's cursor covers a
		// block that has not been applied yet.
		if len(s.holding) > 0 {
			if err := s.flushHolding(s.holding[len(s.holding)-1].cursor); err != nil {
				return fmt.Errorf("flushing held blocks before switching to direct inserts: %w", err)
			}
		}

		if err := s.db.SwitchToDirectInserts(ctx, "stream reached the chain head"); err != nil {
			return fmt.Errorf("switching to direct inserts: %w", err)
		}

		if err := s.applyConstraintsOnce(); err != nil {
			return err
		}

		s.directInserts = true
	}

	holder := &Holder{
		output: output,
		data:   data,
		isLive: isLive,
		cursor: cursor,
	}
	s.holding = append(s.holding, holder)
	s.stats.Progress.RecordDownloaded(data.Clock.Number)
	s.recordBuffered()

	if data.Clock.Number > (s.lastAppliedBlockNum+s.blockBatchSize) || s.blockBatchSize == 1 || (isLive != nil && *isLive) {
		if isLive != nil && *isLive && s.stats.FlushDuration.Average() > data.Clock.Timestamp.AsTime().Sub(s.lastAppliedBlockTime) {
			s.logger.Debug("skipping a flush because we are LIVE and flush average duration is above time between blocks", zapx.HumanDuration("flush_duration_average", s.stats.FlushDuration.Average()), zap.Time("last_block_time", s.lastAppliedBlockTime), zap.Time("block_time", data.Clock.Timestamp.AsTime()))
			return nil
		}

		return s.flushHolding(cursor)
	}

	return nil
}

// HandleBlockRangeCompletion flushes whatever is still held when a bounded run reaches
// its stop block.
//
// Blocks accumulate until the batch is full, so a run that ends mid-batch left its last
// blocks in memory and never wrote them, nor the cursor covering them. With
// --block-batch-size larger than the range, that meant an empty database and a run that
// looked successful.
func (s *Sinker) HandleBlockRangeCompletion(ctx context.Context, cursor *sink.Cursor) error {

	if len(s.holding) > 0 {
		s.logger.Info("flushing blocks held at the end of the requested range", zap.Int("block_count", len(s.holding)))

		if err := s.flushHolding(cursor); err != nil {
			return err
		}
	}

	// The spool still holds whatever has not reached its segment size, and it owns the
	// transactions while it is open. Draining it first is what keeps those blocks from
	// being streamed, and paid for, twice, and what leaves the constraints to be created
	// against a database that already holds every row of the range.
	if err := s.db.SwitchToDirectInserts(ctx, "stream reached the end of the requested range"); err != nil {
		return fmt.Errorf("draining before the end of the range: %w", err)
	}

	if err := s.applyConstraintsOnce(); err != nil {
		return err
	}

	return s.db.Close(ctx)
}

// flushHolding applies every held block, stores the cursor and commits.
func (s *Sinker) flushHolding(cursor *sink.Cursor) (err error) {
	if len(s.holding) == 0 {
		return nil
	}

	lastClock := s.holding[len(s.holding)-1].data.Clock

	// Decode before opening the transaction: it touches no database state, and holding
	// one open across the CPU-bound work would pin a connection for nothing.
	decodedBlocks, err := s.decoder.decodeAll(s.holding, s.db)
	if err != nil {
		return fmt.Errorf("decoding blocks: %w", err)
	}

	if s.useTransaction {
		if err := s.db.BeginTransaction(); err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
	}

	insertDuration, err := s.decoder.apply(decodedBlocks, s.db)
	if err != nil {
		if s.useTransaction {
			s.logger.Error("rolling back transaction", zap.Error(err))
			s.db.RollbackTransaction()
		}
		return fmt.Errorf("applying blocks: %w", err)
	}
	s.recordDecodeStats(decodedBlocks, insertDuration)

	flushDuration, err := s.db.Flush()
	if err != nil {
		return fmt.Errorf("flushing: %w", err)
	}

	var flushDurationPerBlock time.Duration
	if len(s.holding) > 0 {
		flushDurationPerBlock = flushDuration / time.Duration(len(s.holding))
	}
	s.stats.FlushDuration.Add(flushDurationPerBlock)

	s.lastAppliedBlockNum = lastClock.Number
	s.lastAppliedBlockTime = lastClock.Timestamp.AsTime()
	err = s.db.StoreCursor(cursor)
	if err != nil {
		return fmt.Errorf("inserting cursor: %w", err)
	}

	if s.useTransaction {
		if err := s.db.CommitTransaction(); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
	}
	s.holding = s.holding[:0]

	// With a local buffer the rows are only queued at this point, not durable, so the
	// applied mark has to come from what the buffer actually committed.
	if _, _, applied, buffering := s.db.BufferStats(); !buffering {
		s.stats.Progress.RecordApplied(lastClock.Number)
	} else if applied > 0 {
		s.stats.Progress.RecordApplied(applied)
	}
	s.recordBuffered()

	return nil
}

// recordBuffered reports what sits between the stream and the database: blocks held in
// memory for the next flush, plus whatever a local buffer has queued on disk.
func (s *Sinker) recordBuffered() {
	bufferedBlocks, bufferedBytes, _, buffering := s.db.BufferStats()
	if !buffering {
		s.stats.Progress.RecordBuffered(len(s.holding), 0)
		return
	}

	s.stats.Progress.RecordBuffered(len(s.holding)+int(bufferedBlocks), bufferedBytes)
}

// recordDecodeStats folds the per-block timings the workers measured back into the
// shared stats, on this goroutine: Average.Add is not safe for concurrent use.
func (s *Sinker) recordDecodeStats(results []*decoded, insertDuration time.Duration) {
	for _, result := range results {
		if result.empty {
			continue
		}
		s.stats.UnmarshallingDuration.Add(result.unmarshalDuration)
		s.stats.EntitiesInsertDuration.Add(result.walkDuration)
	}
	s.stats.BlockInsertDuration.Add(insertDuration)
}

func (s *Sinker) HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) (err error) {
	lastValidBlockNum := undoSignal.LastValidBlock.Number

	s.logger.Info("Handling undo block signal", zap.Stringer("block", cursor.Block()), zap.Stringer("cursor", cursor))

	// Blocks are held in memory until the batch fills, so the undone ones may not have
	// reached the database yet. Writing them first and deleting after is what keeps a
	// later flush from putting back exactly what the undo removed.
	if err := s.flushHolding(cursor); err != nil {
		return fmt.Errorf("flushing held blocks before an undo: %w", err)
	}

	err = s.db.HandleBlocksUndo(lastValidBlockNum)
	if err != nil {
		return fmt.Errorf("handle blocks undo from %d : %w", lastValidBlockNum, err)
	}

	err = s.db.StoreCursor(cursor)
	if err != nil {
		return fmt.Errorf("inserting cursor: %w", err)
	}

	return nil
}

// applyConstraintsOnce creates the schema's constraints now that the bulk of the loading
// is behind us, which is where they belong: measured through binary COPY, loading with
// foreign keys in place costs 27.7x against 3.3x for building them afterwards.
//
// It runs when the stream reaches the chain head, and again at the end of a bounded run
// that never got there. Both are idempotent — the constraints already in place are left
// alone — so whichever comes first does the work.
func (s *Sinker) applyConstraintsOnce() error {
	if s.constraintsApplied || !s.constraints.ApplyAtHead() {
		if !s.constraintsApplied && s.constraints.Timing == sql.ConstraintsManual && !s.constraints.SkipsEverything() {
			s.constraintsApplied = true
			s.logger.Info("the backfill is done and the schema has no constraints yet. Creating them locks every table while indexes are built and foreign keys validated, " +
				"so it is left to you: run `substreams sink postgres apply-constraints <manifest> --dsn ...` when a maintenance window suits")
		}

		return nil
	}
	s.constraintsApplied = true

	s.logger.Info("backfill is done, creating the schema's constraints as --apply-constraints=head asked. This locks every table while it runs")

	startAt := time.Now()
	if err := applyConstraints(s.db, s.logger); err != nil {
		return err
	}

	s.logger.Debug("constraints created", zap.Duration("duration", time.Since(startAt)))

	return nil
}
