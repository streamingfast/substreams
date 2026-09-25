package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/dustin/go-humanize"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//go:generate go-enum -f=$GOFILE --names

// OnFailure selects what the sink does once every attempt to deliver a block
// has failed.
//
// OnFailureSkip logs the failure, drops the block and moves on to the next
// one. The cursor is not advanced past the dropped block, so a later restart
// replays it. An undo notification is never dropped: it is retried until it
// goes through.
//
// OnFailureExit keeps the block in the pending file, writes a termination
// message and stops the sink with ExitCodeDeliveryFailed. The next start
// delivers the pending block before it opens a Substreams stream.
//
// ENUM(skip, exit)
type OnFailure string

// ExitCodeDeliveryFailed is the process exit status for a delivery failure in
// OnFailureExit mode. It is EX_TEMPFAIL from sysexits: the input was fine,
// try again later.
const ExitCodeDeliveryFailed = 75

// DeliveryFailedError is returned from Sink.Run in OnFailureExit mode. The
// pending block stays on disk for the next start.
type DeliveryFailedError struct {
	Delivery       *DeliveryError
	Kind           DeliveryKind
	FirstAttemptAt time.Time
}

func (e *DeliveryFailedError) Error() string {
	return fmt.Sprintf("%v (failing since %s)", e.Delivery, e.FirstAttemptAt.Format(time.RFC3339))
}

func (e *DeliveryFailedError) Unwrap() error { return e.Delivery }

// TerminationMessage is the one-line JSON written to the termination log so
// an orchestrator can read why the process stopped and since when.
func (e *DeliveryFailedError) TerminationMessage() []byte {
	msg, _ := json.Marshal(map[string]any{
		"reason":           "webhook_delivery_failed",
		"kind":             e.Kind,
		"url":              e.Delivery.URL,
		"block":            e.Delivery.BlockNumber,
		"status":           e.Delivery.StatusCode,
		"attempts":         e.Delivery.Attempts,
		"first_attempt_at": e.FirstAttemptAt.UTC().Format(time.RFC3339),
		"error":            e.Delivery.Err.Error(),
	})
	return msg
}

// Sink represents a webhook sink that sends substream data to HTTP endpoints
type Sink struct {
	webhookURL     string
	undoURL        string
	moduleName     string
	stateFile      string
	pendingFile    string
	onFailure      OnFailure
	terminationLog string
	fingerprint    string
	batchMaxBlocks int
	batchMaxWait   time.Duration
	batch          *openBatch
	client         *Client
	sinker         *sink.Sinker
	decoder        *protodecode.Decoder
	logger         *zap.Logger
}

// openBatch is the batch being filled. It is flushed when it holds
// batchMaxBlocks blocks, when batchMaxWait has passed since it was opened,
// when a live block arrives, when an undo signal arrives, or when the stream
// ends cleanly.
type openBatch struct {
	payload *BatchPayload
	// cursors holds the cursor of each entry in payload.Blocks, so an undo
	// can cut the batch at any block.
	cursors  []string
	openedAt time.Time
}

func (b *openBatch) lastBlock() uint64 {
	return b.payload.Blocks[len(b.payload.Blocks)-1].Clock.Number
}

// cursor is the cursor of the last block that came with one.
func (b *openBatch) cursor() string {
	for i := len(b.cursors) - 1; i >= 0; i-- {
		if b.cursors[i] != "" {
			return b.cursors[i]
		}
	}
	return ""
}

// truncate drops the blocks above lastValid. It reports false when no block
// is left.
func (b *openBatch) truncate(lastValid uint64) bool {
	n := len(b.payload.Blocks)
	for n > 0 && b.payload.Blocks[n-1].Clock.Number > lastValid {
		n--
	}
	b.payload.Blocks = b.payload.Blocks[:n]
	b.cursors = b.cursors[:n]
	return n > 0
}

// SinkConfig holds configuration for the webhook sink
type SinkConfig struct {
	WebhookURL string
	// UndoURL receives an UndoPayload for every undo signal the sink sees.
	// Empty disables the notification; the cursor still moves back so the
	// blocks that replace the undone ones are delivered as usual.
	UndoURL string
	// StateFile holds the cursor of the last delivered block. The pending
	// block lives next to it in "<StateFile>.pending". Empty disables both.
	StateFile    string
	OnFailure    OnFailure
	SinkerConfig *sink.SinkerConfig
	ClientConfig Config
	// BatchMaxBlocks above zero switches every call to the BatchPayload shape
	// and sends up to that many blocks per call. Zero sends one WebhookPayload
	// per block.
	BatchMaxBlocks int
	// BatchMaxWait bounds how long a batch waits for more blocks. It is
	// checked when the next block arrives. Defaults to one second.
	BatchMaxWait time.Duration
	// TerminationLogPath receives the reason for a delivery-failure exit. It
	// is written only when the file already exists, which is the case under
	// Kubernetes, so the default of /dev/termination-log is safe elsewhere.
	TerminationLogPath string
	Logger             *zap.Logger
}

var WebhookCallsCounter = sink.Metrics.NewCounter("substreams_sink_webhook_calls", "Number of calls made to the webhook")
var WebhookSizeBytes = sink.Metrics.NewCounter("substreams_sink_webhook_bytes_sent", "Number of bytes sent via webhook")
var WebhookLastDeliveredBlock = sink.Metrics.NewGauge("substreams_sink_webhook_last_delivered_block", "Last block number delivered to the webhook")

// NewSink creates a new webhook sink
func NewSink(config SinkConfig) (*Sink, error) {
	sinker, err := sink.NewFromConfig(config.SinkerConfig)
	if err != nil {
		return nil, err
	}

	decoder, err := protodecode.NewDecoder(sinker.Package(), []string{sinker.OutputModuleName()})
	if err != nil {
		return nil, fmt.Errorf("creating decoder: %w", err)
	}

	onFailure := config.OnFailure
	if onFailure == "" {
		onFailure = OnFailureSkip
	}

	batchMaxWait := config.BatchMaxWait
	if batchMaxWait <= 0 {
		batchMaxWait = time.Second
	}

	return &Sink{
		batchMaxBlocks: max(config.BatchMaxBlocks, 0),
		batchMaxWait:   batchMaxWait,
		webhookURL:     config.WebhookURL,
		undoURL:        config.UndoURL,
		moduleName:     sinker.OutputModuleName(),
		stateFile:      config.StateFile,
		pendingFile:    pendingFilePath(config.StateFile),
		onFailure:      onFailure,
		terminationLog: config.TerminationLogPath,
		fingerprint:    configFingerprint(config.WebhookURL, config.UndoURL, config.ClientConfig),
		client:         NewClient(config.ClientConfig, config.Logger),
		sinker:         sinker,
		decoder:        decoder,
		logger:         config.Logger,
	}, nil
}

// Run delivers the pending block left by a previous run, if any, then streams
// from the cursor. A Substreams stream is never opened while a pending block
// is undelivered, so retrying against a dead endpoint costs no egress.
func (s *Sink) Run(ctx context.Context) error {
	var startCursor *sink.Cursor
	var err error
	if s.stateFile != "" {
		startCursor, err = sink.ReadCursor(s.stateFile)
		if err != nil {
			return fmt.Errorf("reading cursor: %w", err)
		}
	}

	if startCursor, err = s.recoverPending(ctx, startCursor); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	handlers := sink.NewSinkerFullHandlersWithPartial(
		s.handleBlockScopedData,
		s.handleBlockUndoSignal,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	s.sinker.Run(ctx, startCursor, handlers)
	if err := s.sinker.Err(); err != nil {
		return err
	}

	// A clean end, the stop block was reached. On a shutdown the batch is
	// left alone: its cursor was not saved, so the stream re-sends it.
	if s.batch != nil && ctx.Err() == nil {
		return s.flushBatch(ctx)
	}
	return nil
}

// recoverPending delivers the pending block from a previous run and returns
// the cursor to stream from afterwards.
func (s *Sink) recoverPending(ctx context.Context, startCursor *sink.Cursor) (*sink.Cursor, error) {
	pending, err := readPending(s.pendingFile)
	if err != nil {
		return nil, fmt.Errorf("reading pending delivery: %w", err)
	}
	if pending == nil {
		return startCursor, nil
	}

	if !pending.isUndo() && pending.Batched != s.batching() {
		// The user switched batching on or off while the sink was down. The
		// receiver expects the new shape, and the cursor was not advanced past
		// these blocks, so the stream sends them again in that shape.
		s.logger.Info("pending payload was written in the other batching mode, discarding it; the stream re-sends its blocks",
			zap.Bool("pending_batched", pending.Batched), zap.Bool("batching", s.batching()), zap.Uint64("block", pending.BlockNumber))
		return startCursor, removePending(s.pendingFile)
	}

	if pending.isUndo() && s.undoURL == "" {
		// The undo URL was removed while the sink was down. Without it an undo
		// only moves the cursor back, so do that and drop the notification.
		s.logger.Warn("dropping pending reorg notification, no undo URL is configured", zap.Uint64("last_valid_block", pending.BlockNumber))
		s.commit(pending)
		return pendingCursor(pending)
	}

	if pending.Fingerprint != s.fingerprint {
		// The user changed the URL or a secret since the failure began. What
		// follows is a fresh outage, if it is one at all.
		s.logger.Info("delivery configuration changed since the pending block was written, resetting its first attempt time", zap.Uint64("block", pending.BlockNumber))
		pending.FirstAttemptAt = time.Now()
		pending.Fingerprint = s.fingerprint
		if err := writePending(s.pendingFile, pending); err != nil {
			return nil, fmt.Errorf("updating pending delivery: %w", err)
		}
	}

	s.logger.Info("delivering pending block before connecting to substreams",
		zap.Uint64("block", pending.BlockNumber),
		zap.Time("first_attempt_at", pending.FirstAttemptAt))

	if err := s.deliver(ctx, pending); err != nil {
		if ctx.Err() != nil {
			// A shutdown, not a failed delivery: the pending file stays as is.
			return nil, ctx.Err()
		}
		if s.onFailure == OnFailureExit {
			return nil, s.deliveryFailed(pending, err)
		}
		if pending.isUndo() {
			if err := s.retryUndo(ctx, pending, err); err != nil {
				return nil, err
			}
			return pendingCursor(pending)
		}
		s.logger.Warn("dropping pending block after failed delivery", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
		return startCursor, removePending(s.pendingFile)
	}

	return pendingCursor(pending)
}

func pendingCursor(pending *pendingDelivery) (*sink.Cursor, error) {
	cursor, err := sink.NewCursor(pending.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor in pending delivery: %w", err)
	}
	return cursor, nil
}

func (s *Sink) urlFor(pending *pendingDelivery) string {
	if pending.isUndo() {
		return s.undoURL
	}
	return s.webhookURL
}

// deliver POSTs the pending payload and, on success, commits it. The returned
// error is always the client's delivery error.
func (s *Sink) deliver(ctx context.Context, pending *pendingDelivery) error {
	WebhookCallsCounter.Inc()
	WebhookSizeBytes.AddInt(len(pending.Payload))

	if err := s.client.Call(ctx, s.urlFor(pending), pending.Payload, pending.BlockNumber); err != nil {
		return err
	}

	s.commit(pending)
	return nil
}

// commit records a delivered payload: cursor written, pending file removed,
// progress metric updated. The receiver already has the payload, so a disk
// error here is only logged and never goes through the on-failure policy; the
// worst it causes is the payload being sent again after a restart.
func (s *Sink) commit(pending *pendingDelivery) {
	if s.stateFile != "" && pending.Cursor != "" {
		cursor, err := sink.NewCursor(pending.Cursor)
		if err != nil {
			s.logger.Warn("invalid cursor for delivered payload, state file not updated", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
		} else if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
			s.logger.Warn("failed to save cursor to state file", zap.String("file", s.stateFile), zap.Error(err))
		}
	}
	if err := removePending(s.pendingFile); err != nil {
		s.logger.Warn("failed to remove pending delivery", zap.String("file", s.pendingFile), zap.Error(err))
	}

	WebhookLastDeliveredBlock.SetUint64(pending.BlockNumber)
}

// deliveryFailed keeps the payload on disk for the next start and turns the
// failure into the error Run returns in exit mode, after writing the
// termination message.
func (s *Sink) deliveryFailed(pending *pendingDelivery, err error) error {
	if writeErr := writePending(s.pendingFile, pending); writeErr != nil {
		// Not fatal for the data: the cursor was not advanced, so the stream
		// re-sends this block on the next start. Only the egress saving is lost.
		s.logger.Error("failed to keep the undelivered payload on disk", zap.String("file", s.pendingFile), zap.Error(writeErr))
	}

	var deliveryErr *DeliveryError
	if !errors.As(err, &deliveryErr) {
		deliveryErr = &DeliveryError{URL: s.urlFor(pending), BlockNumber: pending.BlockNumber, Attempts: 1, Err: err}
	}
	kind := pending.Kind
	if kind == "" {
		kind = DeliveryKindBlock
	}
	failed := &DeliveryFailedError{Delivery: deliveryErr, Kind: kind, FirstAttemptAt: pending.FirstAttemptAt}

	if err := writeTerminationMessage(s.terminationLog, failed.TerminationMessage()); err != nil {
		s.logger.Warn("failed to write termination message", zap.String("path", s.terminationLog), zap.Error(err))
	}
	return failed
}

// writeTerminationMessage writes msg to path when path already exists. Under
// Kubernetes the kubelet creates the file; anywhere else nothing is written.
func writeTerminationMessage(path string, msg []byte) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.WriteFile(path, msg, 0o644)
}

func (s *Sink) batching() bool { return s.batchMaxBlocks > 0 }

// handleBlockScopedData processes a block of data and sends it to the webhook
func (s *Sink) handleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	now := time.Now()

	// Blocks with no output do not join a batch, but they do move the clock
	// for one that is waiting: a sparse module must not hold a batch forever.
	if s.batch != nil && now.Sub(s.batch.openedAt) >= s.batchMaxWait {
		if err := s.flushBatch(ctx); err != nil {
			return err
		}
	}

	if data.Output.MapOutput.Value == nil {
		return nil
	}

	msgDesc := s.decoder.GetMessageDescriptor(data.Output.Name)
	dataContent := s.decoder.DecodeDynamicMessage(msgDesc, data.Output.MapOutput)

	live := isLive != nil && *isLive
	if s.batching() {
		return s.addToBatch(ctx, data.Output.Name, data.Output.MapOutput.TypeUrl, data.Clock, dataContent, cursor, live, now)
	}
	return s.sendBlock(ctx, data.Output.Name, data.Output.MapOutput.TypeUrl, data.Clock, dataContent, cursor, now)
}

// sendBlock delivers one block as a WebhookPayload.
func (s *Sink) sendBlock(ctx context.Context, moduleName, typeURL string, clock *pbsubstreams.Clock, dataContent json.RawMessage, cursor *sink.Cursor, now time.Time) error {
	payload, err := NewWebhookPayload(moduleName, clock, typeURL, dataContent)
	if err != nil {
		return fmt.Errorf("failed to create webhook payload: %w", err)
	}

	wrappedOut, err := payload.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize webhook payload: %w", err)
	}

	pending := &pendingDelivery{
		Kind:           DeliveryKindBlock,
		BlockNumber:    clock.Number,
		Payload:        wrappedOut,
		FirstAttemptAt: now,
		Fingerprint:    s.fingerprint,
	}
	if cursor != nil {
		pending.Cursor = cursor.String()
	}

	return s.send(ctx, pending)
}

// addToBatch appends the block to the open batch and flushes it when it is
// full, when it waited long enough, or when the chain is live and holding the
// block back would only add latency.
func (s *Sink) addToBatch(ctx context.Context, moduleName, typeURL string, clock *pbsubstreams.Clock, dataContent json.RawMessage, cursor *sink.Cursor, live bool, now time.Time) error {
	if s.batch == nil {
		s.batch = &openBatch{payload: NewBatchPayload(moduleName, typeURL), openedAt: now}
	}
	s.batch.payload.Append(clock, dataContent)
	var cursorStr string
	if cursor != nil {
		cursorStr = cursor.String()
	}
	s.batch.cursors = append(s.batch.cursors, cursorStr)

	full := len(s.batch.payload.Blocks) >= s.batchMaxBlocks
	waited := now.Sub(s.batch.openedAt) >= s.batchMaxWait
	if full || waited || live {
		return s.flushBatch(ctx)
	}
	return nil
}

// flushBatch delivers the open batch as one call and closes it. The batch
// is closed even when delivery fails: in exit mode it is on disk, in skip mode
// it is dropped.
func (s *Sink) flushBatch(ctx context.Context) error {
	batch := s.batch
	s.batch = nil

	wrappedOut, err := batch.payload.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize batch payload: %w", err)
	}

	pending := &pendingDelivery{
		Kind:           DeliveryKindBlock,
		Batched:        true,
		Cursor:         batch.cursor(),
		BlockNumber:    batch.lastBlock(),
		Payload:        wrappedOut,
		FirstAttemptAt: time.Now(),
		Fingerprint:    s.fingerprint,
	}

	s.logger.Debug("flushing batch", zap.Int("blocks", len(batch.payload.Blocks)), zap.Uint64("last_block", batch.lastBlock()))
	return s.send(ctx, pending)
}

// send delivers the payload and applies the on-failure policy.
func (s *Sink) send(ctx context.Context, pending *pendingDelivery) error {
	s.logger.Info("calling webhook", zap.Stringer("kind", pending.Kind), zap.Uint64("block", pending.BlockNumber))

	err := s.deliver(ctx, pending)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		// A shutdown, not a failed delivery. The cursor was not saved, so the
		// stream re-sends this payload on the next start.
		return ctx.Err()
	}

	if s.onFailure == OnFailureExit {
		return s.deliveryFailed(pending, err)
	}
	if pending.isUndo() {
		return s.retryUndo(ctx, pending, err)
	}

	s.logger.Warn("dropping block after failed delivery", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
	return nil
}

// retryUndo delivers an undo notification that already failed once, retrying
// until it goes through or ctx is done. Dropping it would let the replacement
// blocks reach a receiver that still holds the undone ones. No pending file
// is written: the cursor still points at the undone blocks, so after a
// restart the stream sends the same undo signal again.
func (s *Sink) retryUndo(ctx context.Context, pending *pendingDelivery, err error) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	if s.client.maxInterval > 0 {
		b.MaxInterval = s.client.maxInterval
		if b.InitialInterval > b.MaxInterval {
			b.InitialInterval = b.MaxInterval
		}
		b.Reset()
	}

	rounds := 1
	for {
		s.logger.Warn("undo notification not delivered, retrying until it goes through",
			zap.Uint64("last_valid_block", pending.BlockNumber),
			zap.Int("rounds", rounds),
			zap.Duration("failing_for", time.Since(pending.FirstAttemptAt)),
			zap.Error(err))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(b.NextBackOff()):
		}

		rounds++
		if err = s.deliver(ctx, pending); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// handleBlockUndoSignal moves the cursor back to the last valid block and, when
// an undo URL is configured, tells the receiver which blocks are gone.
func (s *Sink) handleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	// The blocks of the open batch above the last valid one were never
	// delivered, so they are dropped. The rest must reach the receiver before
	// the undo notification, so they go out first.
	if s.batch != nil && undoSignal.LastValidBlock != nil && !s.batch.truncate(undoSignal.LastValidBlock.Number) {
		s.batch = nil
	}
	if s.batch != nil {
		if err := s.flushBatch(ctx); err != nil {
			return err
		}
	}

	if s.undoURL == "" {
		if s.stateFile != "" && cursor != nil {
			if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
				s.logger.Warn("failed to save cursor to state file on undo",
					zap.Error(err),
					zap.String("file", s.stateFile))
			}
		}
		return nil
	}

	payload, err := NewUndoPayload(s.moduleName, undoSignal.LastValidBlock).ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize undo payload: %w", err)
	}

	pending := &pendingDelivery{
		Kind:           DeliveryKindUndo,
		Payload:        payload,
		FirstAttemptAt: time.Now(),
		Fingerprint:    s.fingerprint,
	}
	if undoSignal.LastValidBlock != nil {
		pending.BlockNumber = undoSignal.LastValidBlock.Number
	}
	if cursor != nil {
		pending.Cursor = cursor.String()
	}

	return s.send(ctx, pending)
}

// PrintStats prints final statistics
func (s *Sink) PrintStats() {
	s.sinker.PrintStats()
	fmt.Fprintf(os.Stderr, " • Total Webhook calls: %s\n", humanize.Comma(int64(WebhookCallsCounter.Get())))
	fmt.Fprintf(os.Stderr, " • Total Webhook bytes sent: %s\n", humanize.IBytes(uint64(WebhookSizeBytes.Get())))
}
