package service

import (
	"context"

	"github.com/streamingfast/bstream"
)

type StreamFactoryFunc func(ctx context.Context, h bstream.Handler, startBlockNum int64, stopBlockNum uint64, cursor string, cursorIsTarget bool) (Streamable, error)

type Streamable interface {
	Run(ctx context.Context) error
}
