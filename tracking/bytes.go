package tracking

import (
	"context"
	"sync"
	"time"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type BytesMeter interface {
	AddBytesWritten(n int)
	AddBytesRead(n int)

	BytesWritten() uint64
	BytesRead() uint64

	Launch(ctx context.Context, respFunc substreams.ResponseFunc)
	Send(respFunc substreams.ResponseFunc) error
}

type bytesMeter struct {
	bytesWritten uint64
	bytesRead    uint64

	mu     sync.RWMutex
	logger *zap.Logger
}

func NewBytesMeter() BytesMeter {
	return &bytesMeter{
		bytesWritten: 0,
		bytesRead:    0,
	}
}

func (b *bytesMeter) Launch(ctx context.Context, respFunc substreams.ResponseFunc) {
	logger := reqctx.Logger(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			err := b.Send(respFunc)
			if err != nil {
				logger.Error("unable to send bytes meter", zap.Error(err))
			}
		}
	}
}

func (b *bytesMeter) Send(respFunc substreams.ResponseFunc) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	msg := &pbsubstreams.ModuleProgress_ProcessedBytes_{
		ProcessedBytes: &pbsubstreams.ModuleProgress_ProcessedBytes{
			TotalBytesWritten: b.bytesWritten,
			TotalBytesRead:    b.bytesRead,
		},
	}

	var in []*pbsubstreams.ModuleProgress
	in = append(in, &pbsubstreams.ModuleProgress{
		Name: "bytes",
		Type: msg,
	})

	resp := substreams.NewModulesProgressResponse(in)

	err := respFunc(resp)
	if err != nil {
		return err
	}

	return nil
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

type noopBytesMeter struct{}

func (_ *noopBytesMeter) AddBytesWritten(n int)                                        { return }
func (_ *noopBytesMeter) AddBytesRead(n int)                                           { return }
func (_ *noopBytesMeter) BytesWritten() uint64                                         { return 0 }
func (_ *noopBytesMeter) BytesRead() uint64                                            { return 0 }
func (_ *noopBytesMeter) Launch(ctx context.Context, respFunc substreams.ResponseFunc) {}
func (_ *noopBytesMeter) Send(respFunc substreams.ResponseFunc) error                  { return nil }

var NoopBytesMeter BytesMeter = &noopBytesMeter{}
