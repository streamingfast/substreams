package pubsub

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

// OnFailure selects what the sink does once every attempt to publish a block
// has failed. The unset value is OnFailureExit: a failed publish stops the
// sink, and no block is dropped.
//
// OnFailureExit keeps the block in the pending file, writes a termination
// reason and stops the sink with ExitCodeDeliveryFailed. The next start
// publishes the pending block before it opens a Substreams stream. The
// pending file and this exit match the webhook sink (sink/webhook).
//
// OnFailureSkip logs the failure, drops the block and moves on to the next
// one. The block stays dropped once a later block saves its cursor. A restart
// before that save sends the block again, because the cursor still points at
// the earlier one. An undo notification is never dropped: it is retried until
// it is published, and it is not written to the pending file.
type OnFailure string

const (
	OnFailureSkip OnFailure = "skip"
	OnFailureExit OnFailure = "exit"
)

// ParseOnFailure parses the --pubsub-on-failure flag.
func ParseOnFailure(s string) (OnFailure, error) {
	switch OnFailure(s) {
	case OnFailureSkip, OnFailureExit:
		return OnFailure(s), nil
	default:
		return "", fmt.Errorf("unknown value %q", s)
	}
}

// OnFailureNames is the accepted --pubsub-on-failure values, default first.
func OnFailureNames() []string { return []string{string(OnFailureExit), string(OnFailureSkip)} }

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

// TerminationReason is the one-line JSON written to the termination log so
// an orchestrator can read why the process stopped and since when.
func (e *DeliveryFailedError) TerminationReason() []byte {
	errText := ""
	if e.Delivery != nil && e.Delivery.Err != nil {
		errText = e.Delivery.Err.Error()
	}
	var project, topic string
	var block uint64
	var attempts int
	if e.Delivery != nil {
		project = e.Delivery.Project
		topic = e.Delivery.Topic
		block = e.Delivery.BlockNumber
		attempts = e.Delivery.Attempts
	}
	msg, _ := json.Marshal(map[string]any{
		"reason":           "pubsub_delivery_failed",
		"kind":             e.Kind,
		"project":          project,
		"topic":            topic,
		"block":            block,
		"attempts":         attempts,
		"first_attempt_at": e.FirstAttemptAt.UTC().Format(time.RFC3339),
		"error":            errText,
	})
	return msg
}

// Sink publishes substreams module output to a Google Cloud Pub/Sub topic.
// The message body is WebhookPayload, BatchPayload, and UndoPayload in this
// package. That JSON matches the webhook sink and must stay in sync with
// sink/webhook/payload.go. A module that emits
// sf.substreams.sink.pubsub.v1.Publish is published in the
// substreams-sink-pubsub wire format.
type Sink struct {
	projectID      string
	topicID        string
	publishUndo    bool
	legacyPublish  bool
	moduleName     string
	orderingKey    string
	stateFile      string
	pendingFile    string
	onFailure      OnFailure
	terminationLog string
	fingerprint    string
	batchMaxBlocks int
	batchMaxBytes  int
	batchMaxWait   time.Duration
	maxInterval    time.Duration
	batch          *openBatch
	publisher      Publisher
	close          func() error
	sinker         *sink.Sinker
	decoder        *protodecode.Decoder
	logger         *zap.Logger
}

// openBatch is the batch being filled. It is flushed when it holds
// batchMaxBlocks blocks, before the next block would take it past
// batchMaxBytes, when batchMaxWait has passed since it was opened,
// when a live block arrives, when an undo signal arrives, or when the stream
// ends cleanly.
type openBatch struct {
	payload *BatchPayload
	// cursors holds the cursor of each entry in payload.Blocks, so an undo
	// can cut the batch at any block.
	cursors []string
	// sizes holds the serialized size of each entry in payload.Blocks, and
	// size the serialized size of the whole payload.
	sizes    []int
	size     int
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
	for _, dropped := range b.sizes[n:] {
		b.size -= dropped
	}
	b.payload.Blocks = b.payload.Blocks[:n]
	b.cursors = b.cursors[:n]
	b.sizes = b.sizes[:n]
	return n > 0
}

// SinkConfig holds configuration for the Pub/Sub sink.
type SinkConfig struct {
	// Project is the Google Cloud project. It is optional when Topic is a
	// projects/<project>/topics/<topic> path.
	Project string
	// Topic is a topic id or a full projects/<project>/topics/<topic> path.
	Topic string
	// PublishUndo publishes an undo payload on Topic for every undo signal.
	// When false, the cursor still moves back so the blocks that replace the
	// undone ones are published as usual.
	PublishUndo bool
	// StateFile holds the cursor of the last delivered block. The pending
	// block lives next to it in "<StateFile>.pending". Empty disables both.
	StateFile    string
	OnFailure    OnFailure
	SinkerConfig *sink.SinkerConfig
	Retry        RetryConfig
	// BatchMaxBlocks above zero switches every message to the batch JSON shape
	// and sends up to that many blocks per message. Zero sends one block per
	// message.
	BatchMaxBlocks int
	// BatchMaxBytes above zero bounds the size of a batch payload: a batch is
	// sent before the next block would take it past this many bytes. A block
	// larger than the limit on its own is sent alone. Zero means no limit.
	BatchMaxBytes int
	// BatchMaxWait bounds how long a batch waits for more blocks. It is
	// checked when the next block arrives. Defaults to one second.
	BatchMaxWait time.Duration
	// TerminationLogPath receives the reason for a delivery-failure exit. It
	// is written only when the file already exists, which is the case under
	// Kubernetes, so the default of /dev/termination-log is safe elsewhere.
	TerminationLogPath string
	Logger             *zap.Logger
}

var PublishesCounter = sink.Metrics.NewCounter("substreams_sink_pubsub_publishes", "Number of successful publishes to Pub/Sub")
var PublishedBytes = sink.Metrics.NewCounter("substreams_sink_pubsub_bytes_sent", "Number of bytes published to Pub/Sub")
var LastDeliveredBlock = sink.Metrics.NewGauge("substreams_sink_pubsub_last_delivered_block", "Last block number published to Pub/Sub")

// NewSink creates a Pub/Sub sink and dials the topic. ctx is the client's
// lifetime, not the stream's: cancel the context passed to Run to stop
// streaming without closing the client. Call Close when finished.
func NewSink(ctx context.Context, config SinkConfig) (*Sink, error) {
	projectID, topicID, err := ParseTopic(config.Project, config.Topic)
	if err != nil {
		return nil, err
	}

	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	sinker, err := sink.NewFromConfig(config.SinkerConfig)
	if err != nil {
		return nil, err
	}
	decoder, err := protodecode.NewDecoder(sinker.Package(), []string{sinker.OutputModuleName()})
	if err != nil {
		return nil, fmt.Errorf("creating decoder: %w", err)
	}

	publisher, err := NewGCPPublisher(ctx, projectID, topicID, config.Retry, logger)
	if err != nil {
		return nil, err
	}

	return newSink(config, projectID, topicID, publisher, publisher.Close, sinker, decoder, logger), nil
}

func newSink(config SinkConfig, projectID, topicID string, publisher Publisher, closeFn func() error, sinker *sink.Sinker, decoder *protodecode.Decoder, logger *zap.Logger) *Sink {
	onFailure := config.OnFailure
	if onFailure == "" {
		onFailure = OnFailureExit
	}
	batchMaxWait := config.BatchMaxWait
	if batchMaxWait <= 0 {
		batchMaxWait = time.Second
	}

	moduleName := ""
	legacyPublish := false
	if sinker != nil {
		moduleName = sinker.OutputModuleName()
		legacyPublish = isLegacyPublishType(sinker.OutputModuleTypeUnprefixed())
	}

	return &Sink{
		projectID:      projectID,
		topicID:        topicID,
		publishUndo:    config.PublishUndo,
		legacyPublish:  legacyPublish,
		moduleName:     moduleName,
		orderingKey:    moduleName,
		stateFile:      config.StateFile,
		pendingFile:    pendingFilePath(config.StateFile),
		onFailure:      onFailure,
		terminationLog: config.TerminationLogPath,
		fingerprint:    configFingerprint(projectID, topicID, config.PublishUndo),
		batchMaxBlocks: max(config.BatchMaxBlocks, 0),
		batchMaxBytes:  max(config.BatchMaxBytes, 0),
		batchMaxWait:   batchMaxWait,
		maxInterval:    config.Retry.MaxInterval,
		publisher:      publisher,
		close:          closeFn,
		sinker:         sinker,
		decoder:        decoder,
		logger:         logger,
	}
}

// Close releases the Pub/Sub client. It is safe to call more than once.
func (s *Sink) Close() error {
	if s.close == nil {
		return nil
	}
	err := s.close()
	s.close = nil
	return err
}

// Run publishes the pending block left by a previous run, if any, then streams
// from the cursor. A Substreams stream is never opened while a pending block
// is unpublished, so retrying against an unreachable topic costs no egress.
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

// recoverPending publishes the pending block from a previous run and returns
// the cursor to stream from afterwards.
func (s *Sink) recoverPending(ctx context.Context, startCursor *sink.Cursor) (*sink.Cursor, error) {
	pending, err := readPending(s.pendingFile)
	if err != nil {
		return nil, fmt.Errorf("reading pending delivery: %w", err)
	}
	if pending == nil {
		return startCursor, nil
	}

	if pending.Legacy {
		// The messages are already in the substreams-sink-pubsub wire format.
		// Batching flags and --pubsub-undo do not change that format, so a
		// restart publishes the same messages, including a reorg.
		s.legacyPublish = true
	}

	if !pending.Legacy && !pending.isUndo() && pending.Batched != s.batching() {
		// The user switched batching on or off while the sink was down. The
		// subscriber expects the new shape, and the cursor was not advanced
		// past these blocks, so the stream sends them again in that shape.
		s.logger.Info("pending payload was written in the other batching mode, discarding it; the stream re-sends its blocks",
			zap.Bool("pending_batched", pending.Batched), zap.Bool("batching", s.batching()), zap.Uint64("block", pending.BlockNumber))
		return startCursor, removePending(s.pendingFile)
	}

	if pending.isUndo() && !pending.Legacy && !s.publishUndo {
		// Undo publishing was turned off while the sink was down. Without it
		// an undo only moves the cursor back, so do that and drop the
		// notification.
		s.logger.Warn("dropping pending reorg notification, undo publishing is disabled", zap.Uint64("last_valid_block", pending.BlockNumber))
		s.commit(pending)
		return pendingCursor(pending)
	}

	if pending.Fingerprint != s.fingerprint {
		// The user changed the topic or the undo setting since the failure
		// began. What follows is a fresh outage, if it is one at all.
		s.logger.Info("delivery configuration changed since the pending block was written, resetting its first attempt time", zap.Uint64("block", pending.BlockNumber))
		pending.FirstAttemptAt = time.Now()
		pending.Fingerprint = s.fingerprint
		if err := writePending(s.pendingFile, pending); err != nil {
			return nil, fmt.Errorf("updating pending delivery: %w", err)
		}
	}

	s.logger.Info("publishing pending block before connecting to substreams",
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

// deliver publishes the pending payload and, on success, commits it. The
// returned error is the publisher's delivery error.
func (s *Sink) deliver(ctx context.Context, pending *pendingDelivery) error {
	for _, msg := range pending.outbound(s.orderingKey) {
		err := s.publisher.Publish(ctx, msg)
		if err != nil {
			var delivery *DeliveryError
			if errors.As(err, &delivery) {
				delivery.BlockNumber = pending.BlockNumber
				if delivery.Project == "" {
					delivery.Project = s.projectID
				}
				if delivery.Topic == "" {
					delivery.Topic = s.topicID
				}
			}
			return err
		}
		// Count a publish only after it succeeds. Retries inside Publish are not
		// counted again.
		PublishesCounter.Inc()
		PublishedBytes.AddInt(len(msg.Data))
	}

	s.commit(pending)
	return nil
}

// outbound is the Pub/Sub messages for one delivery. Legacy messages already
// carry their attributes and ordering key. The webhook JSON is one message.
func (p *pendingDelivery) outbound(orderingKey string) []Message {
	if p.Legacy {
		return p.Messages
	}
	return []Message{{
		Data:        p.Payload,
		Attributes:  map[string]string{AttributeType: p.messageType()},
		OrderingKey: orderingKey,
	}}
}

// commit records a published payload: cursor written, pending file removed,
// progress metric updated. The subscriber already has the payload, so a disk
// error here is only logged and never goes through the on-failure policy; the
// worst it causes is the payload being sent again after a restart.
func (s *Sink) commit(pending *pendingDelivery) {
	if s.stateFile != "" && pending.Cursor != "" {
		cursor, err := sink.NewCursor(pending.Cursor)
		if err != nil {
			s.logger.Warn("invalid cursor for published payload, state file not updated", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
		} else if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
			s.logger.Warn("failed to save cursor to state file", zap.String("file", s.stateFile), zap.Error(err))
		}
	}
	if err := removePending(s.pendingFile); err != nil {
		s.logger.Warn("failed to remove pending delivery", zap.String("file", s.pendingFile), zap.Error(err))
	}

	LastDeliveredBlock.SetUint64(pending.BlockNumber)
}

// deliveryFailed keeps the payload on disk for the next start and turns the
// failure into the error Run returns in exit mode, after writing the
// termination reason.
func (s *Sink) deliveryFailed(pending *pendingDelivery, err error) error {
	if writeErr := writePending(s.pendingFile, pending); writeErr != nil {
		// Not fatal for the data: the cursor was not advanced, so the stream
		// re-sends this block on the next start. Only the egress saving is lost.
		s.logger.Error("failed to keep the undelivered payload on disk", zap.String("file", s.pendingFile), zap.Error(writeErr))
	}

	var deliveryErr *DeliveryError
	if !errors.As(err, &deliveryErr) {
		deliveryErr = &DeliveryError{Project: s.projectID, Topic: s.topicID, BlockNumber: pending.BlockNumber, Attempts: 1, Err: err}
	}
	kind := pending.Kind
	if kind == "" {
		kind = DeliveryKindBlock
	}
	failed := &DeliveryFailedError{Delivery: deliveryErr, Kind: kind, FirstAttemptAt: pending.FirstAttemptAt}

	if err := writeTerminationReason(s.terminationLog, failed.TerminationReason()); err != nil {
		s.logger.Warn("failed to write termination reason", zap.String("path", s.terminationLog), zap.Error(err))
	}
	return failed
}

// writeTerminationReason writes msg to path when path already exists. Under
// Kubernetes the kubelet creates the file; anywhere else nothing is written.
func writeTerminationReason(path string, msg []byte) error {
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

// handleBlockScopedData publishes one block, or appends it to the open batch.
func (s *Sink) handleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	now := time.Now()

	// Blocks with no output do not join a batch, but they do move the clock
	// for one that is waiting: a sparse module must not hold a batch forever.
	if s.batch != nil && now.Sub(s.batch.openedAt) >= s.batchMaxWait {
		if err := s.flushBatch(ctx); err != nil {
			return err
		}
	}

	// MapOutput is nil when the block has nothing to publish. --noop-mode
	// sends an empty MapModuleOutput, so reading Value unguarded panics.
	if data.Output == nil || data.Output.MapOutput == nil || data.Output.MapOutput.Value == nil {
		return nil
	}

	if isLegacyPublishType(data.Output.MapOutput.TypeUrl) {
		s.legacyPublish = true
		return s.sendLegacyBlock(ctx, data.Clock, data.Output.MapOutput.Value, cursor, now)
	}

	msgDesc := s.decoder.GetMessageDescriptor(data.Output.Name)
	dataContent := s.decoder.DecodeDynamicMessage(msgDesc, data.Output.MapOutput)

	live := isLive != nil && *isLive
	if s.batching() {
		return s.addToBatch(ctx, data.Output.Name, data.Output.MapOutput.TypeUrl, data.Clock, dataContent, cursor, live, now)
	}
	return s.sendBlock(ctx, data.Output.Name, data.Output.MapOutput.TypeUrl, data.Clock, dataContent, cursor, now)
}

// sendLegacyBlock publishes one block the way substreams-sink-pubsub does.
// Batching does not apply: each Publish.Message is its own Pub/Sub message.
func (s *Sink) sendLegacyBlock(ctx context.Context, clock *pbsubstreams.Clock, raw []byte, cursor *sink.Cursor, now time.Time) error {
	var blockNum uint64
	if clock != nil {
		blockNum = clock.Number
	}
	var cursorStr string
	if cursor != nil {
		cursorStr = cursor.String()
	}
	messages, err := legacyBlockMessages(blockNum, cursorStr, raw)
	if err != nil {
		return err
	}
	return s.send(ctx, &pendingDelivery{
		Kind:           DeliveryKindBlock,
		Legacy:         true,
		Messages:       messages,
		BlockNumber:    blockNum,
		Cursor:         cursorStr,
		FirstAttemptAt: now,
		Fingerprint:    s.fingerprint,
	})
}

// sendLegacyUndo publishes the substreams-sink-pubsub undo message. That sink
// always emits it, so --pubsub-undo is not required for a Publish module.
func (s *Sink) sendLegacyUndo(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	var blockNum uint64
	if undoSignal.LastValidBlock != nil {
		blockNum = undoSignal.LastValidBlock.Number
	}
	var cursorStr string
	if cursor != nil {
		cursorStr = cursor.String()
	}
	return s.send(ctx, &pendingDelivery{
		Kind:           DeliveryKindUndo,
		Legacy:         true,
		Messages:       []Message{legacyUndoMessage(blockNum, cursorStr)},
		BlockNumber:    blockNum,
		Cursor:         cursorStr,
		FirstAttemptAt: time.Now(),
		Fingerprint:    s.fingerprint,
	})
}

// sendBlock publishes one block as a WebhookPayload.
func (s *Sink) sendBlock(ctx context.Context, moduleName, typeURL string, clock *pbsubstreams.Clock, dataContent json.RawMessage, cursor *sink.Cursor, now time.Time) error {
	payload, err := NewWebhookPayload(moduleName, clock, typeURL, dataContent)
	if err != nil {
		return fmt.Errorf("creating block payload: %w", err)
	}
	wrappedOut, err := payload.ToJSON()
	if err != nil {
		return fmt.Errorf("serializing block payload: %w", err)
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
	single, err := NewWebhookPayload(moduleName, clock, typeURL, dataContent)
	if err != nil {
		return fmt.Errorf("creating batch entry: %w", err)
	}
	entry := BlockEntry{Clock: single.Clock, Data: dataContent}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("serializing batch entry: %w", err)
	}
	// Entries after the first are preceded by a comma.
	entrySize := len(encoded) + 1

	if s.batch != nil && s.batchMaxBytes > 0 && s.batch.size+entrySize > s.batchMaxBytes {
		if err := s.flushBatch(ctx); err != nil {
			return err
		}
	}

	if s.batch == nil {
		payload := NewBatchPayload(moduleName, typeURL)
		payload.Blocks = []BlockEntry{}
		empty, err := payload.ToJSON()
		if err != nil {
			return fmt.Errorf("serializing batch payload: %w", err)
		}
		s.batch = &openBatch{payload: payload, size: len(empty) - 1, openedAt: now} // the first entry needs no comma
	}
	s.batch.payload.Blocks = append(s.batch.payload.Blocks, entry)
	var cursorStr string
	if cursor != nil {
		cursorStr = cursor.String()
	}
	s.batch.cursors = append(s.batch.cursors, cursorStr)
	s.batch.sizes = append(s.batch.sizes, entrySize)
	s.batch.size += entrySize

	full := len(s.batch.payload.Blocks) >= s.batchMaxBlocks || (s.batchMaxBytes > 0 && s.batch.size >= s.batchMaxBytes)
	waited := now.Sub(s.batch.openedAt) >= s.batchMaxWait
	if full || waited || live {
		return s.flushBatch(ctx)
	}
	return nil
}

// flushBatch publishes the open batch as one message and closes it. The batch
// is closed even when publishing fails: in exit mode it is on disk, in skip
// mode it is dropped.
func (s *Sink) flushBatch(ctx context.Context) error {
	batch := s.batch
	s.batch = nil

	wrappedOut, err := batch.payload.ToJSON()
	if err != nil {
		return fmt.Errorf("serializing batch payload: %w", err)
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

// send publishes the payload and applies the on-failure policy.
func (s *Sink) send(ctx context.Context, pending *pendingDelivery) error {
	s.logger.Info("publishing",
		zap.String("type", pending.messageType()),
		zap.Uint64("block", pending.BlockNumber),
		zap.String("topic", s.topicID))

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

	s.logger.Warn("dropping block after failed publish", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
	return nil
}

// retryUndo publishes an undo notification that already failed once, retrying
// until it goes through or ctx is done. Dropping it would let the replacement
// blocks reach a subscriber that still holds the undone ones. No pending file
// is written: the cursor still points at the undone blocks, so after a
// restart the stream sends the same undo signal again.
func (s *Sink) retryUndo(ctx context.Context, pending *pendingDelivery, err error) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	if s.maxInterval > 0 {
		b.MaxInterval = s.maxInterval
		if b.InitialInterval > b.MaxInterval {
			b.InitialInterval = b.MaxInterval
		}
		b.Reset()
	}

	rounds := 1
	for {
		s.logger.Warn("undo notification not published, retrying until it goes through",
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

// handleBlockUndoSignal moves the cursor back to the last valid block.
// A Publish module always emits the substreams-sink-pubsub reorg message.
// Any other module emits the webhook undo JSON when undo publishing is on.
func (s *Sink) handleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	// The blocks of the open batch above the last valid one were never
	// published, so they are dropped. The rest must reach the subscriber
	// before the undo notification, so they go out first.
	if s.batch != nil && undoSignal.LastValidBlock != nil && !s.batch.truncate(undoSignal.LastValidBlock.Number) {
		s.batch = nil
	}
	if s.batch != nil {
		if err := s.flushBatch(ctx); err != nil {
			return err
		}
	}

	if s.legacyPublish {
		return s.sendLegacyUndo(ctx, undoSignal, cursor)
	}

	if !s.publishUndo {
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
		return fmt.Errorf("serializing undo payload: %w", err)
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

// PrintStats prints final statistics.
func (s *Sink) PrintStats() {
	if s.sinker != nil {
		s.sinker.PrintStats()
	}
	fmt.Fprintf(os.Stderr, " • Total PubSub publishes: %s\n", humanize.Comma(int64(PublishesCounter.Get())))
	fmt.Fprintf(os.Stderr, " • Total PubSub bytes sent: %s\n", humanize.IBytes(uint64(PublishedBytes.Get())))
}
