package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// OnFailure selects what the sink does once every attempt to deliver a block
// has failed.
type OnFailure string

const (
	// OnFailureSkip logs the failure, drops the block and moves on to the next
	// one. The cursor is not advanced past the dropped block, so a later
	// restart replays it.
	OnFailureSkip OnFailure = "skip"
	// OnFailureExit keeps the block in the pending file, writes a termination
	// message and stops the sink with ExitCodeDeliveryFailed. The next start
	// delivers the pending block before it opens a Substreams stream.
	OnFailureExit OnFailure = "exit"
)

func ParseOnFailure(value string) (OnFailure, error) {
	switch OnFailure(value) {
	case OnFailureSkip, OnFailureExit:
		return OnFailure(value), nil
	default:
		return "", fmt.Errorf("invalid on-failure value %q, expected %q or %q", value, OnFailureSkip, OnFailureExit)
	}
}

// ExitCodeDeliveryFailed is the process exit status for a delivery failure in
// OnFailureExit mode. It is EX_TEMPFAIL from sysexits: the input was fine,
// try again later.
const ExitCodeDeliveryFailed = 75

// DeliveryFailedError is returned from Sink.Run in OnFailureExit mode. The
// pending block stays on disk for the next start.
type DeliveryFailedError struct {
	Delivery *DeliveryError
	// Kind is "block" for a block payload and "undo" for a reorg notification.
	Kind           string
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
	client         *Client
	sinker         *sink.Sinker
	decoder        *protodecode.Decoder
	logger         *zap.Logger
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
	// TerminationLogPath receives the reason for a delivery-failure exit. It
	// is written only when the file already exists, which is the case under
	// Kubernetes, so the default of /dev/termination-log is safe elsewhere.
	TerminationLogPath string
	Logger             *zap.Logger
}

var WebhookCallsCounter = sink.Metrics.NewCounter("webhook_calls", "Number of calls made to the webhook")
var WebhookSizeBytes = sink.Metrics.NewCounter("webhook_bytes_sent", "Number of bytes sent via webhook")
var WebhookProgressBlock = sink.Metrics.NewGauge("substreams_sink_progress_block", "Last block number delivered to the webhook")

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

	return &Sink{
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
	return s.sinker.Err()
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
		if s.onFailure == OnFailureExit {
			return nil, s.deliveryFailed(pending, err)
		}
		s.logger.Warn("dropping pending block after failed delivery", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
		return startCursor, removePending(s.pendingFile)
	}

	cursor, err := sink.NewCursor(pending.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor in pending delivery: %w", err)
	}
	return cursor, nil
}

// deliver POSTs the pending payload and, on success, commits it: cursor
// written, pending file removed, progress metric updated. The returned error
// is the *DeliveryError from the client.
func (s *Sink) deliver(ctx context.Context, pending *pendingDelivery) error {
	url := s.webhookURL
	if pending.isUndo() {
		url = s.undoURL
	}

	WebhookCallsCounter.Inc()
	WebhookSizeBytes.AddInt(len(pending.Payload))

	if err := s.client.Call(ctx, url, pending.Payload, pending.BlockNumber); err != nil {
		return err
	}

	if s.stateFile != "" && pending.Cursor != "" {
		cursor, err := sink.NewCursor(pending.Cursor)
		if err != nil {
			return fmt.Errorf("invalid cursor for block %d: %w", pending.BlockNumber, err)
		}
		if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
			return fmt.Errorf("saving cursor to state file %q: %w", s.stateFile, err)
		}
	}
	if err := removePending(s.pendingFile); err != nil {
		return fmt.Errorf("removing pending delivery: %w", err)
	}

	WebhookProgressBlock.SetUint64(pending.BlockNumber)
	return nil
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
		deliveryErr = &DeliveryError{URL: s.webhookURL, BlockNumber: pending.BlockNumber, Attempts: 1, Err: err}
	}
	kind := pending.Kind
	if kind == "" {
		kind = pendingKindBlock
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

// handleBlockScopedData processes a block of data and sends it to the webhook
func (s *Sink) handleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	if data.Output.MapOutput.Value == nil {
		return nil
	}

	msgDesc := s.decoder.GetMessageDescriptor(data.Output.Name)
	dataContent := s.decoder.DecodeDynamicMessage(msgDesc, data.Output.MapOutput)

	payload, err := NewWebhookPayload(data.Output.Name, data.Clock, data.Output.MapOutput.TypeUrl, dataContent)
	if err != nil {
		return fmt.Errorf("failed to create webhook payload: %w", err)
	}

	wrappedOut, err := payload.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize webhook payload: %w", err)
	}

	pending := &pendingDelivery{
		Kind:           pendingKindBlock,
		BlockNumber:    data.Clock.Number,
		Payload:        wrappedOut,
		FirstAttemptAt: time.Now(),
		Fingerprint:    s.fingerprint,
	}
	if cursor != nil {
		pending.Cursor = cursor.String()
	}

	return s.send(ctx, pending)
}

// send delivers the payload and applies the on-failure policy.
func (s *Sink) send(ctx context.Context, pending *pendingDelivery) error {
	s.logger.Info("calling webhook", zap.String("kind", pending.Kind), zap.Uint64("block", pending.BlockNumber))

	err := s.deliver(ctx, pending)
	if err == nil {
		return nil
	}

	if s.onFailure == OnFailureExit {
		return s.deliveryFailed(pending, err)
	}

	s.logger.Warn("dropping block after failed delivery", zap.Uint64("block", pending.BlockNumber), zap.Error(err))
	return nil
}

// handleBlockUndoSignal moves the cursor back to the last valid block and, when
// an undo URL is configured, tells the receiver which blocks are gone.
func (s *Sink) handleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
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
		Kind:           pendingKindUndo,
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
