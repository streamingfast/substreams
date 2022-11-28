package reqctx

import (
	"context"
	"sync"
)

type IBytesMeter interface {
	AddBytesWritten(n int)
	AddBytesRead(n int)

	BytesWritten() uint64
	BytesRead() uint64
}

type bytesMeter struct {
	bytesWritten uint64
	bytesRead    uint64

	mu sync.RWMutex
}

func NewBytesMeter() IBytesMeter {
	return &bytesMeter{
		bytesWritten: 0,
		bytesRead:    0,
	}
}

func (b *bytesMeter) AddBytesWritten(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n < 0 {
		panic("negative value")
	}

	b.bytesWritten += uint64(n)
}

func (b *bytesMeter) AddBytesRead(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n < 0 {
		panic("negative value")
	}

	b.bytesRead += uint64(n)
}

func (b *bytesMeter) BytesWritten() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesWritten
}

func (b *bytesMeter) BytesRead() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesRead
}

type NoopBytesMeter struct{}

func (_ *NoopBytesMeter) AddBytesWritten(n int) { return }
func (_ *NoopBytesMeter) AddBytesRead(n int)    { return }
func (_ *NoopBytesMeter) BytesWritten() uint64  { return 0 }
func (_ *NoopBytesMeter) BytesRead() uint64     { return 0 }

var noopBytesMeter = &NoopBytesMeter{}

func WithBytesMeter(ctx context.Context, meter IBytesMeter) context.Context {
	return context.WithValue(ctx, bytesMeterKey, meter)
}

func BytesMeter(ctx context.Context) IBytesMeter {
	meter := ctx.Value(bytesMeterKey)
	if m, ok := meter.(IBytesMeter); ok {
		return m
	}
	return noopBytesMeter
}
