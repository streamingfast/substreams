package sinker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
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

// FinalFlushTimeout bounds the retries of the flush of the handled blocks when the stream is stopped. An
// attempt already writing is not interrupted.
const FinalFlushTimeout = 40 * time.Second

// RunStopTimeout is how long to wait for Run to return once the sinker is stopped, see [sink.Sinker.AwaitRun].
// Past it the process exits and the flush transaction is rolled back, the sink then re-processes the blocks
// since the stored cursor on restart. On Kubernetes, the pod's terminationGracePeriodSeconds (30s by default)
// must be raised above it, or the pod is killed first, with the same outcome.
const RunStopTimeout = FinalFlushTimeout + 5*time.Second

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

	// writerLockPollInterval, when non-zero, makes Run take the writer lock first, waiting as a
	// standby while another sink holds it (see db.Loader.AcquireWriterLock).
	writerLockPollInterval time.Duration

	// stallTimeout, when non-zero, stops the sink if no block arrives for that long while live.
	stallTimeout time.Duration
	lastBlockAt  atomic.Int64 // unix nanoseconds
	live         atomic.Bool

	// lastHandledCursor is the cursor of the last block or undo given to this sink, and
	// lastFinalBlockHeight the last final block height the stream reported with a block. With an undo
	// buffer, the stream's own cursor can be ahead of the last handled one.
	lastHandledCursor    *sink.Cursor
	lastFinalBlockHeight uint64
	// handlerFailed is set once a block or undo failed to be handled: pending changes may then hold
	// part of a block, so they must not be flushed.
	handlerFailed bool

	// runStarted and runDone track Run as a whole, standby wait and lock release included (see AwaitRun).
	runStarted atomic.Bool
	runDone    chan struct{}
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
		runDone:             make(chan struct{}),
	}, nil
}

// Close stops the sinker, waits for Run to return if it was started, then closes the database,
// releasing the writer lock if held.
func (s *SQLSinker) Close() error {
	s.logger.Info("closing SQL sinker")
	s.Shutdown(nil)
	_ = s.AwaitRun(context.Background())

	if err := s.loader.Close(); err != nil {
		return fmt.Errorf("loader close: %w", err)
	}
	return nil
}

// AwaitRun waits for Run to return, or for ctx to be done, then returns the first failure of the base
// sinker's Run. It also covers what Run does around the stream, standing by for the writer lock and
// releasing it, which the base sinker's AwaitRun does not see. It returns right away if Run was not
// started.
func (s *SQLSinker) AwaitRun(ctx context.Context) error {
	if s.runStarted.Load() {
		select {
		case <-s.runDone:
		case <-ctx.Done():
			return fmt.Errorf("sinker run did not finish, its last writes may or may not have completed: %w", ctx.Err())
		}
	}
	return s.Sinker.AwaitRun(ctx)
}

// SetWriterLock makes Run take the writer lock of the cursors table before loading the cursor. While
// another sink holds it, this one waits as a standby, checking every pollInterval. PostgreSQL only.
func (s *SQLSinker) SetWriterLock(pollInterval time.Duration) {
	s.writerLockPollInterval = pollInterval
}

// SetStallTimeout makes the sink stop with an error if no block arrives for timeout while live, so a
// supervisor restarts it (and, with the writer lock, a standby can take over). Zero disables it.
func (s *SQLSinker) SetStallTimeout(timeout time.Duration) {
	s.stallTimeout = timeout
}

func (s *SQLSinker) Run(ctx context.Context) {
	s.runStarted.Store(true)
	defer close(s.runDone)

	// Cancelled as soon as this sinker terminates, whatever stage Run is at: standing by, loading the
	// cursor, or streaming (the stream loop gets this context too).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.OnTerminating(func(_ error) { cancel() })
	if s.IsTerminating() {
		return
	}

	if s.writerLockPollInterval > 0 {
		if !s.acquireWriterLock(ctx) {
			return
		}
		stopKeepalive := s.keepWriterSessionAlive(ctx)
		// Released once the stream loop below has returned, so never under a running flush.
		defer func() {
			stopKeepalive()
			s.loader.ReleaseWriterLock()
			WriterLockHeld.SetUint64(0)
		}()
	}

	cursor, rawCursor, mismatchDetected, err := s.loader.GetCursor(ctx, s.OutputModuleHash())
	if err != nil && !errors.Is(err, db2.ErrCursorNotFound) {
		s.Shutdown(fmt.Errorf("unable to retrieve cursor: %w", err))
		return
	}

	// We write an empty cursor right away in the database because the flush logic
	// only performs an `update` operation so an initial cursor is required in the database
	// for the flush to work correctly.
	if errors.Is(err, db2.ErrCursorNotFound) {
		blank := sink.NewBlankCursor()
		if err := s.loader.InsertCursor(ctx, s.OutputModuleHash(), blank); err != nil {
			s.Shutdown(fmt.Errorf("unable to write initial empty cursor: %w", err))
			return
		}
		rawCursor = blank.String()

	} else if mismatchDetected {
		if err := s.loader.InsertCursor(ctx, s.OutputModuleHash(), cursor); err != nil {
			s.Shutdown(fmt.Errorf("unable to write new cursor after module mismatch: %w", err))
			return
		}
		rawCursor = cursor.String()
	}

	// From here on, each flush and revert first checks that the stored cursor is still the one read or
	// written above, or the one this sink wrote since.
	s.loader.SetExpectedCursor(rawCursor)

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

	// A stop that came before the callbacks above were registered must still stop the base sinker
	if s.IsTerminating() {
		s.Sinker.Shutdown(s.Err())
		return
	}

	logEach := 15 * time.Second
	if s.logger.Core().Enabled(zap.DebugLevel) {
		logEach = 5 * time.Second
	}

	s.stats.Start(logEach, cursor)

	if s.stallTimeout > 0 {
		s.lastBlockAt.Store(time.Now().UnixNano())
		go s.watchStall()
	}

	s.logger.Info("starting sql sink",
		zapx.HumanDuration("stats_refresh_each", logEach),
		zap.Stringer("restarting_at", cursor.Block()),
		zap.String("loader", s.loader.GetIdentifier()),
	)
	s.Sinker.Run(ctx, cursor, s)
}

// acquireWriterLock waits as a standby until the writer lock is held, and reports whether it is.
// The metrics endpoint is served meanwhile, so standbys report the writer lock gauge.
func (s *SQLSinker) acquireWriterLock(ctx context.Context) bool {
	s.Sinker.ServeMetrics()
	WriterLockHeld.SetUint64(0)

	err := s.loader.AcquireWriterLock(ctx, s.writerLockPollInterval, s.Terminating())
	if s.IsTerminating() || ctx.Err() != nil {
		s.loader.ReleaseWriterLock() // stopping while standing by
		s.Shutdown(nil)
		return false
	}
	if err != nil {
		s.Shutdown(fmt.Errorf("unable to take the writer lock: %w", err))
		return false
	}

	WriterLockHeld.SetUint64(1)
	return true
}

// keepWriterSessionAlive queries the session holding the writer lock every poll interval while no
// flush is running, so long stretches without flushes don't let a proxy or idle_session_timeout end
// it (and hand the lock to a standby). If the session is gone, the sink stops: a standby may already
// be writing. The returned function stops the keepalive and waits for it.
func (s *SQLSinker) keepWriterSessionAlive(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.writerLockPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			err := s.loader.KeepWriterSessionAlive(ctx, max(s.writerLockPollInterval, 5*time.Second))
			switch {
			case err == nil || ctx.Err() != nil:
			case db2.IsCancellation(err):
				// Timed out: the database is slow, but the session may well be alive; the next
				// flush (or the stall timeout) will tell.
				s.logger.Warn("writer lock session keepalive timed out", zap.Error(err))
			default:
				s.Shutdown(fmt.Errorf("lost the database session holding the writer lock: %w", err))
				return
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
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
		if errors.Is(err, db2.ErrCursorConflict) {
			// Retrying cannot help: the sink must restart from the cursor stored in the database.
			return 0, err
		}
		if s.loader.HoldsWriterLock() && !s.loader.WriterSessionAlive(ctx) {
			// The writer lock went with its session; a standby may already be writing.
			return 0, fmt.Errorf("lost the database session holding the writer lock: %w", err)
		}
		lastErr = err
	}

	return 0, fmt.Errorf("flush failed after %d retries: %w", retries, lastErr)
}

// watchStall stops the sink if no block arrives for stallTimeout while live. A sink stuck on a
// silent stream or a hung database would otherwise keep running (and keep the writer lock).
func (s *SQLSinker) watchStall() {
	interval := max(min(s.stallTimeout/4, 5*time.Second), 10*time.Millisecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.Terminating():
			return
		case <-ticker.C:
			// The stream's own record, taken before any undo buffer, counts blocks the buffer keeps
			// back; it stops advancing while a handler call (a database write) hangs.
			last := time.Unix(0, s.lastBlockAt.Load())
			if received := s.Sinker.LastBlockMessageAt(); received.After(last) {
				last = received
			}
			idle := time.Since(last)
			if s.live.Load() && idle > s.stallTimeout {
				s.Shutdown(fmt.Errorf("no block received for %s while live (--stall-timeout %s), stopping so the sink can be restarted", idle.Round(time.Second), s.stallTimeout))
				return
			}
		}
	}
}

func (s *SQLSinker) recordBlockReceived(isLive *bool) {
	s.lastBlockAt.Store(time.Now().UnixNano())
	if isLive != nil {
		s.live.Store(*isLive)
	}
}

func (s *SQLSinker) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) (err error) {
	blockReceivedAt := time.Now()
	s.recordBlockReceived(isLive)
	defer func() {
		if err != nil {
			s.handlerFailed = true
			return
		}
		s.lastHandledCursor = cursor
		s.lastFinalBlockHeight = data.FinalBlockHeight
	}()

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
	if s.handlerFailed {
		// Pending changes may hold part of a block: drop them, the sink resumes from the stored cursor.
		s.logger.Info("a block failed to be handled, not flushing pending changes")
		return nil
	}

	// The stream also ends without error when it is stopped.
	stopped := s.IsTerminating() || ctx.Err() != nil

	if !stopped {
		// To be moved in the base sinker library, happens usually only on integration tests where the connection
		// can close with "nil" error but we haven't completed the range for real yet.
		// The stop block is exclusive: the last streamed block is stopBlock-1, which
		// completes the range and must trigger the final flush.
		streamCursor := cursor
		if received := s.Sinker.LastReceivedCursor(); received != nil {
			streamCursor = received
		}
		stopBlock := s.Sinker.StopBlock()
		if stopBlock > 0 && streamCursor.Block().Num() < stopBlock-1 {
			s.logger.Debug("range not completed yet, skipping", zap.Stringer("block", streamCursor.Block()), zap.Uint64("stop_block", stopBlock))
			return nil
		}
	}

	// Flush with the cursor of the last block this sink handled and the final block height the stream
	// reported. The stream's own cursor can be ahead of the handled blocks (undo buffer), and its block
	// is not necessarily final: treating it as final would prune the undo history of blocks that are
	// still reversible.
	if s.lastHandledCursor == nil {
		return nil
	}
	if stopped {
		s.logger.Info("stream stopped, flushing handled blocks to database", zap.Stringer("block", s.lastHandledCursor.Block()))
		// The stream's context is cancelled already: give this last flush its own, bounded one.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), FinalFlushTimeout)
		defer cancel()
	} else {
		s.logger.Info("stream completed, flushing to database", zap.Stringer("block", s.lastHandledCursor.Block()))
	}
	if _, err := s.flushWithRetry(ctx, s.OutputModuleHash(), s.lastHandledCursor, s.lastFinalBlockHeight, s.flushRetryCount); err != nil {
		return fmt.Errorf("failed to flush %s block on completion: %w", s.lastHandledCursor.Block(), err)
	}

	return nil
}

func (s *SQLSinker) HandleBlockUndoSignal(ctx context.Context, data *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	handlerStart := time.Now()
	s.recordBlockReceived(nil)

	err := s.loader.Revert(ctx, s.OutputModuleHash(), cursor, data.LastValidBlock.Number)
	if err != nil {
		s.handlerFailed = true
		return err
	}
	s.lastHandledCursor = cursor

	handleUndoDuration := time.Since(handlerStart)
	s.stats.RecordHandleUndoDuration(handleUndoDuration)

	return nil
}

func (s *SQLSinker) batchBlockModulo(isLive *bool) uint64 {
	if isLive == nil {
		panic(fmt.Errorf("liveness checker has been disabled on the Sinker instance, this is invalid in the context of 'substreams sink postgres|clickhouse run'"))
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
