package sinker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	"github.com/streamingfast/shutter"
	sink "github.com/streamingfast/substreams/sink"
	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const BLOCK_FLUSH_INTERVAL_DISABLED = 0

type SQLSinker struct {
	*shutter.Shutter
	*sink.Sinker

	loader *db2.Loader
	logger *zap.Logger
	tracer logging.Tracer

	stats                *Stats
	lastAppliedBlockNum  uint64
	lastAppliedBlockTime time.Time

	flushRetryCount int
	flushRetryDelay time.Duration
}

func New(sink *sink.Sinker, loader *db2.Loader, logger *zap.Logger, tracer logging.Tracer, flushRetryCount int, flushRetryDelay time.Duration) (*SQLSinker, error) {
	return &SQLSinker{
		Shutter: shutter.New(),
		Sinker:  sink,

		loader: loader,
		logger: logger,
		tracer: tracer,

		stats:               NewStats(logger),
		lastAppliedBlockNum: 0,
		flushRetryCount:     flushRetryCount,
		flushRetryDelay:     flushRetryDelay,
	}, nil
}

func (s *SQLSinker) Close() error {
	if s.IsTerminated() {
		return nil
	}

	s.logger.Info("closing SQL sinker")
	if err := s.loader.Close(); err != nil {
		return fmt.Errorf("loader close: %w", err)
	}

	s.Shutdown(nil)
	return nil
}

func (s *SQLSinker) Run(ctx context.Context) {
	cursor, mismatchDetected, err := s.loader.GetCursor(ctx, s.OutputModuleHash())
	if err != nil && !errors.Is(err, db2.ErrCursorNotFound) {
		s.Shutdown(fmt.Errorf("unable to retrieve cursor: %w", err))
		return
	}

	// We write an empty cursor right away in the database because the flush logic
	// only performs an `update` operation so an initial cursor is required in the database
	// for the flush to work correctly.
	if errors.Is(err, db2.ErrCursorNotFound) {
		if err := s.loader.InsertCursor(ctx, s.OutputModuleHash(), sink.NewBlankCursor()); err != nil {
			s.Shutdown(fmt.Errorf("unable to write initial empty cursor: %w", err))
			return
		}

	} else if mismatchDetected {
		if err := s.loader.InsertCursor(ctx, s.OutputModuleHash(), cursor); err != nil {
			s.Shutdown(fmt.Errorf("unable to write new cursor after module mismatch: %w", err))
			return
		}
	}

	// Works in all cases, even if the cursor is blank or nil (gives 0)
	s.lastAppliedBlockNum = cursor.Block().Num()

	s.Sinker.OnTerminating(s.Shutdown)
	s.OnTerminating(func(err error) {
		s.stats.LogNow()
		s.logger.Info("sql sinker terminating", zap.Stringer("last_block_written", s.stats.lastBlock))
		s.Sinker.Shutdown(err)
	})

	s.OnTerminating(func(_ error) { s.stats.Close() })
	s.stats.OnTerminated(func(err error) { s.Shutdown(err) })

	logEach := 15 * time.Second
	if s.logger.Core().Enabled(zap.DebugLevel) {
		logEach = 5 * time.Second
	}

	s.stats.Start(logEach, cursor)

	s.logger.Info("starting sql sink",
		zapx.HumanDuration("stats_refresh_each", logEach),
		zap.Stringer("restarting_at", cursor.Block()),
		zap.String("loader", s.loader.GetIdentifier()),
	)
	s.Sinker.Run(ctx, cursor, s)
}

func (s *SQLSinker) flushWithRetry(ctx context.Context, moduleHash string, cursor *sink.Cursor, finalBlockHeight uint64, retries int) (int, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			// Do not retry if flush delay is 0, useful in tests
			if s.flushRetryDelay == 0 {
				return 0, lastErr
			}

			delay := time.Duration(attempt) * s.flushRetryDelay
			s.logger.Warn("retrying flush after error",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", retries),
				zapx.HumanDuration("delay", delay),
				zap.Error(lastErr))

			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(delay):
			}
		}

		rowCount, err := s.loader.Flush(ctx, moduleHash, cursor, finalBlockHeight)
		if err == nil {
			if attempt > 0 {
				s.logger.Info("flush succeeded after retry", zap.Int("attempt", attempt))
			}
			return rowCount, nil
		}
		lastErr = err
	}

	return 0, fmt.Errorf("flush failed after %d retries: %w", retries, lastErr)
}

func (s *SQLSinker) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	blockReceivedAt := time.Now()

	output := data.Output

	if output.Name == "" {
		return nil
	}

	if output.Name != s.OutputModuleName() {
		return fmt.Errorf("received data from wrong output module, expected to received from %q but got module's output for %q", s.OutputModuleName(), output.Name)
	}

	dbChanges := &pbdatabase.DatabaseChanges{}
	mapOutput := output.GetMapOutput()

	if mapOutput.String() != "" {
		if !mapOutput.MessageIs(dbChanges) && mapOutput.TypeUrl != "type.googleapis.com/sf.substreams.database.v1.DatabaseChanges" {
			return fmt.Errorf("mismatched message type: trying to unmarshal unknown type %q", mapOutput.MessageName())
		}

		// We do not use UnmarshalTo here because we need to parse an older proto type and
		// UnmarshalTo enforces the type check. So we check manually the `TypeUrl` above and we use
		// `Unmarshal` instead which only deals with the bytes value.
		if err := proto.Unmarshal(mapOutput.Value, dbChanges); err != nil {
			return fmt.Errorf("unmarshal database changes: %w", err)
		}

		if err := s.applyDatabaseChanges(dbChanges, data.Clock.Number, data.FinalBlockHeight); err != nil {
			return fmt.Errorf("apply database changes: %w", err)
		}
	}

	batchModulo := s.batchBlockModulo(isLive)
	blockFlushNeeded := batchModulo > 0 && data.Clock.Number-s.lastAppliedBlockNum >= batchModulo

	s.logger.Debug("flush condition evaluation",
		zap.Uint64("batch_modulo", batchModulo),
		zap.Uint64("current_block", data.Clock.Number),
		zap.Uint64("last_applied_block", s.lastAppliedBlockNum),
		zap.Uint64("block_diff", data.Clock.Number-s.lastAppliedBlockNum),
		zap.Bool("block_flush_needed_before_timing_check", blockFlushNeeded))

	if blockFlushNeeded && isLive != nil && *isLive && s.stats.AverageFlushDuration() > data.Clock.Timestamp.AsTime().Sub(s.lastAppliedBlockTime) {
		s.logger.Debug("skipping a flush because we are LIVE and flush average duration is above time between blocks", zapx.HumanDuration("flush_duration_average", s.stats.AverageFlushDuration()), zap.Time("last_block_time", s.lastAppliedBlockTime), zap.Time("block_time", data.Clock.Timestamp.AsTime()))
		blockFlushNeeded = false
	}

	rowFlushNeeded := s.loader.FlushNeeded()
	s.logger.Debug("final flush decision",
		zap.Bool("block_flush_needed", blockFlushNeeded),
		zap.Bool("row_flush_needed", rowFlushNeeded))

	if blockFlushNeeded || rowFlushNeeded {
		s.logger.Debug("flushing to database",
			zap.Stringer("block", cursor.Block()),
			zap.Uint64("last_flushed_block", s.lastAppliedBlockNum),
			zap.Bool("is_live", *isLive),
			zap.Bool("block_flush_interval_reached", blockFlushNeeded),
			zap.Bool("row_flush_interval_reached", rowFlushNeeded),
		)

		flushStart := time.Now()
		rowFlushedCount, err := s.flushWithRetry(ctx, s.OutputModuleHash(), cursor, data.FinalBlockHeight, s.flushRetryCount)
		if err != nil {
			return fmt.Errorf("failed to flush at block %s: %w", cursor.Block(), err)
		}

		flushDuration := time.Since(flushStart)
		handleBlockDuration := time.Since(blockReceivedAt)

		if flushDuration > 5*time.Second {
			level := zap.InfoLevel
			if flushDuration > 30*time.Second {
				level = zap.WarnLevel
			}

			s.logger.Check(level, "flush to database took a long time to complete, could cause long sync time along the road").Write(zapx.HumanDuration("took", flushDuration))
		}

		FlushCount.Inc()
		FlushedRowsCount.AddInt(rowFlushedCount)
		FlushDuration.AddInt64(flushDuration.Nanoseconds())
		FlushedHeadBlockTimeDrift.SetBlockTime(data.Clock.GetTimestamp().AsTime())
		FlushedHeadBlockNumber.SetUint64(data.Clock.GetNumber())

		s.stats.RecordBlock(cursor.Block())
		s.stats.RecordFlushDuration(flushDuration)
		s.stats.RecordHandleBlockDuration(handleBlockDuration)
		s.lastAppliedBlockNum = data.Clock.Number
		s.lastAppliedBlockTime = data.Clock.Timestamp.AsTime()
	}

	return nil
}

func (s *SQLSinker) applyDatabaseChanges(dbChanges *pbdatabase.DatabaseChanges, blockNum, finalBlockNum uint64) error {
	for _, change := range dbChanges.TableChanges {
		if !s.loader.HasTable(change.Table) {
			return fmt.Errorf(
				"your Substreams sent us a change for a table named %s we don't know about on %s (available tables: %s)",
				change.Table,
				s.loader.GetIdentifier(),
				strings.Join(s.loader.GetAvailableTablesInSchema(), ", "),
			)
		}

		var primaryKeys map[string]string
		switch u := change.PrimaryKey.(type) {
		case *pbdatabase.TableChange_Pk:
			var err error
			primaryKeys, err = s.loader.GetPrimaryKey(change.Table, u.Pk)
			if err != nil {
				return err
			}
		case *pbdatabase.TableChange_CompositePk:
			primaryKeys = u.CompositePk.Keys
		default:
			return fmt.Errorf("unknown primary key type: %T", change.PrimaryKey)
		}

		changes := map[string]db2.FieldData{}
		for _, field := range change.Fields {
			changes[field.Name] = db2.FieldData{
				Value:    field.Value,
				UpdateOp: protoUpdateOpToDbUpdateOp(field.UpdateOp),
			}
		}

		var reversibleBlockNum *uint64
		if blockNum > finalBlockNum {
			reversibleBlockNum = &blockNum
		}

		switch change.Operation {
		case pbdatabase.TableChange_OPERATION_CREATE:
			err := s.loader.Insert(change.Table, primaryKeys, changes, reversibleBlockNum)
			if err != nil {
				return fmt.Errorf("database insert: %w", err)
			}
		case pbdatabase.TableChange_OPERATION_UPSERT:
			err := s.loader.Upsert(change.Table, primaryKeys, changes, reversibleBlockNum)
			if err != nil {
				return fmt.Errorf("database upsert: %w", err)
			}
		case pbdatabase.TableChange_OPERATION_UPDATE:
			err := s.loader.Update(change.Table, primaryKeys, changes, reversibleBlockNum)
			if err != nil {
				return fmt.Errorf("database update: %w", err)
			}
		case pbdatabase.TableChange_OPERATION_DELETE:
			err := s.loader.Delete(change.Table, primaryKeys, reversibleBlockNum)
			if err != nil {
				return fmt.Errorf("database delete: %w", err)
			}
		default:
		}
	}

	return nil
}

// protoUpdateOpToDbUpdateOp converts proto Field_UpdateOp to db UpdateOp
func protoUpdateOpToDbUpdateOp(op pbdatabase.Field_UpdateOp) db2.UpdateOp {
	switch op {
	case pbdatabase.Field_UPDATE_OP_ADD:
		return db2.UpdateOpAdd
	case pbdatabase.Field_UPDATE_OP_MAX:
		return db2.UpdateOpMax
	case pbdatabase.Field_UPDATE_OP_MIN:
		return db2.UpdateOpMin
	case pbdatabase.Field_UPDATE_OP_SET_IF_NULL:
		return db2.UpdateOpSetIfNull
	default:
		return db2.UpdateOpSet
	}
}

func (s *SQLSinker) HandleBlockRangeCompletion(ctx context.Context, cursor *sink.Cursor) error {
	// To be moved in the base sinker library, happens usually only on integration tests where the connection
	// can close with "nil" error but we haven't completed the range for real yet.
	stopBlock := s.Sinker.StopBlock()
	if stopBlock > 0 && cursor.Block().Num() < stopBlock {
		s.logger.Debug("range not completed yet, skipping", zap.Stringer("block", cursor.Block()), zap.Uint64("stop_block", stopBlock))
		return nil
	}

	s.logger.Info("stream completed, flushing to database", zap.Stringer("block", cursor.Block()))
	_, err := s.flushWithRetry(ctx, s.OutputModuleHash(), cursor, cursor.Block().Num(), s.flushRetryCount)
	if err != nil {
		return fmt.Errorf("failed to flush %s block on completion: %w", cursor.Block(), err)
	}

	return nil
}

func (s *SQLSinker) HandleBlockUndoSignal(ctx context.Context, data *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	handlerStart := time.Now()

	err := s.loader.Revert(ctx, s.OutputModuleHash(), cursor, data.LastValidBlock.Number)
	if err != nil {
		return err
	}

	handleUndoDuration := time.Since(handlerStart)
	s.stats.RecordHandleUndoDuration(handleUndoDuration)

	return nil
}

func (s *SQLSinker) batchBlockModulo(isLive *bool) uint64 {
	if isLive == nil {
		panic(fmt.Errorf("liveness checker has been disabled on the Sinker instance, this is invalid in the context of 'substreams-sink-sql'"))
	}

	if *isLive {
		return uint64(s.loader.LiveBlockFlushInterval())
	}

	if s.loader.BatchBlockFlushInterval() > 0 {
		return uint64(s.loader.BatchBlockFlushInterval())
	}

	return BLOCK_FLUSH_INTERVAL_DISABLED
}

func ptr[T any](v T) *T {
	return &v
}
