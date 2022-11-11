package service

import (
	"context"

	"github.com/streamingfast/bstream"
)

type StreamFactoryFunc func(h bstream.Handler, startBlockNum int64, stopBlockNum uint64, cursor string) (Streamable, error)

type Streamable interface {
	Run(ctx context.Context) error
}
