package webhook

import (
	"context"
	"fmt"

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

// NewSink creates a new webhook sink
func NewSink(config SinkConfig) (*Sink, error) {
	sinker := sink.New(config.SinkerConfig)

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

	handlers := sink.NewSinkerHandlers(
		s.handleBlockScopedData,
		s.handleBlockUndoSignal,
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
}
