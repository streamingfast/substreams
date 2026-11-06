package writer

import (
	"context"
	"io"

	"github.com/streamingfast/dstore"

	"github.com/streamingfast/bstream"
)

type Writer interface {
	io.Writer

	IsWritten() bool
	StartBoundary(*bstream.Range) error
	CloseBoundary(ctx context.Context) (Uploadeable, error)
	Type() FileType
}

type Uploadeable interface {
	Upload(ctx context.Context, store dstore.Store) (string, error)
}
