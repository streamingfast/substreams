package noop

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// Sink represents a noop sink that only handles cursors without processing data
type Sink struct {
	stateFile string
	sinker    *sink.Sinker
	logger    *zap.Logger
}

// SinkConfig holds configuration for the noop sink
type SinkConfig struct {
	StateFile    string
	SinkerConfig *sink.SinkerConfig
	Logger       *zap.Logger
}

// NewSink creates a new noop sink
func NewSink(config SinkConfig) (*Sink, error) {

	// This is usually a configuration rejected by the sinker, only useful in a noop situation
	config.SinkerConfig.SupportIndexOutputProductionMode = true

	sinker, err := sink.NewFromConfig(config.SinkerConfig)
	if err != nil {
		return nil, fmt.Errorf("creating sink: %w", err)
	}

	return &Sink{
		stateFile: config.StateFile,
		sinker:    sinker,
		logger:    config.Logger,
	}, nil
}

// Run starts the noop sink
func (s *Sink) Run(ctx context.Context) error {
	// Load existing cursor if state file exists
	var startCursor *sink.Cursor
	if s.stateFile != "" {
		if data, err := ioutil.ReadFile(s.stateFile); err == nil {
			cursorStr := strings.TrimSpace(string(data))
			if cursorStr != "" {
				if cursor, err := sink.NewCursor(cursorStr); err == nil {
					startCursor = cursor
					s.logger.Info("loaded cursor from state file",
						zap.String("cursor", cursorStr),
						zap.String("file", s.stateFile))
				} else {
					s.logger.Warn("failed to parse cursor from state file",
						zap.Error(err),
						zap.String("file", s.stateFile))
				}
			}
		}
	}

	handlers := sink.NewSinkerHandlers(
		s.handleBlockScopedData,
		s.handleBlockUndoSignal,
	)

	s.sinker.Run(ctx, startCursor, handlers)
	return s.sinker.Err()
}

// handleBlockScopedData processes a block of data but does nothing with it (noop)
func (s *Sink) handleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	// Save cursor to state file without processing the data
	if s.stateFile != "" && cursor != nil {
		if err := s.saveCursor(cursor); err != nil {
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
		if err := s.saveCursor(cursor); err != nil {
			s.logger.Warn("failed to save cursor to state file on undo",
				zap.Error(err),
				zap.String("file", s.stateFile))
		}
	}
	return nil
}

// saveCursor saves the cursor to the state file
func (s *Sink) saveCursor(cursor *sink.Cursor) error {
	cursorStr := cursor.String()
	return ioutil.WriteFile(s.stateFile, []byte(cursorStr), 0644)
}

// PrintStats prints final statistics
func (s *Sink) PrintStats() {
	s.sinker.PrintStats()
}
