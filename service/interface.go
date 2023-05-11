package service

import (
	"context"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
)

type StreamFactoryFunc func(ctx context.Context,
	h bstream.Handler,
	startBlockNum int64,
	stopBlockNum uint64,
	cursor string,
	finalBlocksOnly bool,
	cursorIsTarget bool,
	logger *zap.Logger) (Streamable, error)

type Streamable interface {
	Run(ctx context.Context) error
}
