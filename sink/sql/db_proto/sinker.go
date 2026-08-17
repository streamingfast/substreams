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
	useConstraints        bool
	lastAppliedBlockNum   uint64
	lastAppliedBlockTime  time.Time

	// holding buffers the blocks received since the last flush. It is only ever touched
	// from the sinker's callbacks, which are called sequentially.
	holding []*Holder

	decoder *decoder
}

// NewSinker builds the from-proto sinker. decodeWorkers bounds how many blocks are
// unmarshalled and walked concurrently at flush time; zero picks one per core, less one,
// capped at eight.
func NewSinker(rootMessageDescriptor protoreflect.MessageDescriptor, sink *sink.Sinker, db sql.Database, useTransaction bool, useConstraints bool, blockBatchSize int, decodeWorkers int, stats *stats.Stats, logger *zap.Logger) *Sinker {
	return &Sinker{
		db:                    db,
		rootMessageDescriptor: rootMessageDescriptor,
		useTransaction:        useTransaction,
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

	cursor, err := s.db.FetchCursor()
	if err != nil {
		return fmt.Errorf("fetch cursor: %w", err)
	}

	//clean up the mess from running without a transaction
	if cursor != nil {
		err = s.db.HandleBlocksUndo(cursor.Block().Num())
		if err != nil {
			return fmt.Errorf("handle blocks undo from %s: %w", cursor.Block(), err)
		}
	}
	//panic("Testing 12 12")
	s.logger.Info("fetched cursor", zap.Stringer("block", cursor.Block()))

	s.stats.LastBlockProcessAt = time.Now()
	s.Sinker.Run(ctx, cursor, s)

	return s.Sinker.Err()
}

func (s *Sinker) LogStats() {
	s.stats.Log()
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

	if (isLive != nil && *isLive) && s.useConstraints {
		return fmt.Errorf("live mode is not supported without constraints")
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

	holder := &Holder{
		output: output,
		data:   data,
		isLive: isLive,
		cursor: cursor,
	}
	s.holding = append(s.holding, holder)
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
	if len(s.holding) == 0 {
		return nil
	}

	s.logger.Info("flushing blocks held at the end of the requested range", zap.Int("block_count", len(s.holding)))

	return s.flushHolding(cursor)
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

	return nil
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
