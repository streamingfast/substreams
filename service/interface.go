package service

import (
	"context"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/stream"
	"go.uber.org/zap"
)

type StreamFactoryFunc func(ctx context.Context,
	h bstream.Handler,
	startBlockNum int64,
	stopBlockNum uint64,
	cursor string,
	finalBlocksOnly bool,
	cursorIsTarget bool,
	logger *zap.Logger,
	extraOpts ...stream.Option) (Streamable, error)

type Streamable interface {
	Run(ctx context.Context) error
}
