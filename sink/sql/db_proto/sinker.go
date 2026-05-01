package db_proto

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/logging/zapx"
	sink "github.com/streamingfast/substreams/sink"
	sql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/stats"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type Sinker struct {
	*sink.Sinker
	db                    sql.Database
	useTransaction        bool
	parallel              bool
	blockBatchSize        uint64
	stats                 *stats.Stats
	logger                *zap.Logger
	rootMessageDescriptor protoreflect.MessageDescriptor
	useConstraints        bool
	flushLock             sync.Mutex
	lastAppliedBlockNum   uint64
	lastAppliedBlockTime  time.Time
}

func NewSinker(rootMessageDescriptor protoreflect.MessageDescriptor, sink *sink.Sinker, db sql.Database, useTransaction bool, useConstraints bool, blockBatchSize int, parallel bool, stats *stats.Stats, logger *zap.Logger) *Sinker {
	return &Sinker{
		db:                    db,
		rootMessageDescriptor: rootMessageDescriptor,
		useTransaction:        useTransaction,
		parallel:              parallel,
		blockBatchSize:        uint64(blockBatchSize),
		stats:                 stats,
		Sinker:                sink,
		logger:                logger,
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

var holding []*Holder

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
	holding = append(holding, holder)
	if data.Clock.Number > (s.lastAppliedBlockNum+s.blockBatchSize) || s.blockBatchSize == 1 || (isLive != nil && *isLive) {
		if isLive != nil && *isLive && s.stats.FlushDuration.Average() > data.Clock.Timestamp.AsTime().Sub(s.lastAppliedBlockTime) {
			s.logger.Debug("skipping a flush because we are LIVE and flush average duration is above time between blocks", zapx.HumanDuration("flush_duration_average", s.stats.FlushDuration.Average()), zap.Time("last_block_time", s.lastAppliedBlockTime), zap.Time("block_time", data.Clock.Timestamp.AsTime()))
			return nil
		}

		if s.useTransaction && !s.parallel {
			if err := s.db.BeginTransaction(); err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
		}
		errs := multiError{}
		if s.parallel {
			wg := sync.WaitGroup{}
			wg.Add(len(holding))

			for _, h := range holding {
				go func() {
					db := s.db.Clone()
					err := db.BeginTransaction()
					if err != nil {
						errs = append(errs, err)
					}

					err = s.processHolder(h, s.stats)
					if err != nil {
						db.RollbackTransaction()
						errs = append(errs, err)
						//return fmt.Errorf("process holder: %w", err)
					}
					err = db.CommitTransaction()
					if err != nil {
						errs = append(errs, err)
					}
					wg.Done()
				}()
			}
			wg.Wait()
			if len(errs) > 0 {
				return fmt.Errorf("errors: %w", errs)
			}

		} else {
			for _, h := range holding {
				err = s.processHolder(h, s.stats)
				if err != nil {
					if s.useTransaction {
						s.logger.Error("rolling back transaction", zap.Error(err))
						s.db.RollbackTransaction()
					}
					return fmt.Errorf("process holder: %w", err)
				}
			}
		}

		flushDuration, err := s.db.Flush()
		if err != nil {
			return fmt.Errorf("flushing: %w", err)
		}

		var flushDurationPerBlock time.Duration
		if len(holding) > 0 {
			flushDurationPerBlock = flushDuration / time.Duration(len(holding))
		}
		s.stats.FlushDuration.Add(flushDurationPerBlock)

		s.lastAppliedBlockNum = data.Clock.Number
		s.lastAppliedBlockTime = data.Clock.Timestamp.AsTime()
		err = s.db.StoreCursor(cursor)
		if err != nil {
			return fmt.Errorf("inserting cursor: %w", err)
		}

		if s.useTransaction && !s.parallel {
			if err := s.db.CommitTransaction(); err != nil {
				return fmt.Errorf("commit tx: %w", err)
			}
		}
		holding = []*Holder{}
	}

	return nil
}

func (s *Sinker) processHolder(h *Holder, stats *stats.Stats) (err error) {
	if len(h.output.GetMapOutput().GetValue()) == 0 {
		return nil
	}

	unmarshalStartAt := time.Now()
	md := s.rootMessageDescriptor
	dm := dynamicpb.NewMessage(md)
	err = proto.Unmarshal(h.data.Output.GetMapOutput().GetValue(), dm)
	if err != nil {
		return fmt.Errorf("unmarshaling message: %w", err)
	}

	stats.UnmarshallingDuration.Add(time.Since(unmarshalStartAt))

	err = processMessage(dm, s.db, h.data.Clock.Number, h.data.Clock.Id, h.data.Clock.Timestamp.AsTime(), stats)
	if err != nil {
		return fmt.Errorf("process entity: %w", err)
	}

	return nil
}
func processMessage(dm *dynamicpb.Message, database sql.Database, blockNum uint64, blockHash string, blockTimestamp time.Time, stats *stats.Stats) error {
	startInsertBlock := time.Now()
	err := database.InsertBlock(blockNum, blockHash, blockTimestamp)
	if err != nil {
		return fmt.Errorf("inserting block: %w", err)
	}
	stats.BlockInsertDuration.Add(time.Since(startInsertBlock))

	sqlDuration, err := database.WalkMessageDescriptorAndInsert(dm, blockNum, blockTimestamp, nil)
	if err != nil {
		return fmt.Errorf("processing message %q: %w", string(dm.Descriptor().FullName()), err)
	}

	stats.EntitiesInsertDuration.Add(sqlDuration)

	return nil
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

type multiError []error

func (e multiError) Error() string {
	var msgs []string
	for _, err := range e {
		if err != nil {
			msgs = append(msgs, err.Error())
		}
	}
	return strings.Join(msgs, "\n")
}
