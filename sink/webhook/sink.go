package webhook

import (
	"context"
	"fmt"
	"os"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/substreams/protodecode"
	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// Sink represents a webhook sink that sends substream data to HTTP endpoints
type Sink struct {
	webhookURL string
	stateFile  string
	client     *Client
	sinker     *sink.Sinker
	decoder    *protodecode.Decoder
	logger     *zap.Logger
}

// SinkConfig holds configuration for the webhook sink
type SinkConfig struct {
	WebhookURL   string
	StateFile    string
	SinkerConfig *sink.SinkerConfig
	ClientConfig Config
	Logger       *zap.Logger
}

var WebhookCallsCounter = sink.Metrics.NewCounter("webhook_calls", "Number of calls made to the webhook")
var WebhookSizeBytes = sink.Metrics.NewCounter("webhook_bytes_sent", "Number of bytes sent via webhook")

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

	client := NewClient(config.ClientConfig, config.Logger)

	return &Sink{
		webhookURL: config.WebhookURL,
		stateFile:  config.StateFile,
		client:     client,
		sinker:     sinker,
		decoder:    decoder,
		logger:     config.Logger,
	}, nil
}

// Run starts the webhook sink
func (s *Sink) Run(ctx context.Context) error {
	// Load existing cursor if state file exists
	var startCursor *sink.Cursor
	var err error
	if s.stateFile != "" {
		startCursor, err = sink.ReadCursor(s.stateFile)
		if err != nil {
			return fmt.Errorf("reading cursor: %w", err)
		}
	}

	handlers := sink.NewSinkerFullHandlersWithPartial(
		s.handleBlockScopedData,
		s.handlePartialBlockData,
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

	WebhookCallsCounter.Inc()
	WebhookSizeBytes.AddInt(len(wrappedOut))

	s.logger.Info("calling webhook",
		zap.Uint64("block", data.Clock.Number),
	)

	// Make the webhook call with automatic retries for transient failures
	err = s.client.Call(ctx, s.webhookURL, wrappedOut, data.Clock.Number)
	if err != nil {
		// Continue processing even if webhook fails to avoid blocking the stream
		return nil
	}

	// Save cursor to state file
	if s.stateFile != "" && cursor != nil {
		if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
			s.logger.Warn("failed to save cursor to state file",
				zap.Error(err),
				zap.String("file", s.stateFile))
		}
	}

	return nil
}

// handlePartialBlockData processes a block of partial data and sends it to the webhook
func (s *Sink) handlePartialBlockData(ctx context.Context, partial *pbsubstreamsrpc.PartialBlockData) error {
	if partial.Output.MapOutput.Value == nil {
		return nil
	}

	msgDesc := s.decoder.GetMessageDescriptor(partial.Output.Name)
	dataContent := s.decoder.DecodeDynamicMessage(msgDesc, partial.Output.MapOutput)

	payload, err := NewWebhookPayload(partial.Output.Name, partial.Clock, partial.Output.MapOutput.TypeUrl, dataContent)
	if err != nil {
		return fmt.Errorf("failed to create webhook payload: %w", err)
	}

	wrappedOut, err := payload.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize webhook payload: %w", err)
	}

	WebhookCallsCounter.Inc()
	WebhookSizeBytes.AddInt(len(wrappedOut))

	s.logger.Info("calling webhook",
		zap.Uint64("block", partial.Clock.Number),
	)

	// Make the webhook call with automatic retries for transient failures
	err = s.client.Call(ctx, s.webhookURL, wrappedOut, partial.Clock.Number)
	if err != nil {
		// Continue processing even if webhook fails to avoid blocking the stream
		return nil
	}

	return nil
}

// handleBlockUndoSignal handles undo signals
func (s *Sink) handleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	// Save cursor to state file on undo
	if s.stateFile != "" && cursor != nil {
		if err := sink.WriteCursor(s.stateFile, cursor); err != nil {
			s.logger.Warn("failed to save cursor to state file on undo",
				zap.Error(err),
				zap.String("file", s.stateFile))
		}
	}
	return nil
}

// PrintStats prints final statistics
func (s *Sink) PrintStats() {
	s.sinker.PrintStats()
	fmt.Fprintf(os.Stderr, " • Total Webhook calls: %s\n", humanize.Comma(int64(WebhookCallsCounter.Get())))
	fmt.Fprintf(os.Stderr, " • Total Webhook bytes sent: %s\n", humanize.IBytes(uint64(WebhookSizeBytes.Get())))
}
