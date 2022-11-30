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
	AddBytesWritten(module string, n int)
	AddBytesRead(module string, n int)

	BytesWritten(module string) uint64
	BytesRead(module string) uint64

	Launch(ctx context.Context, respFunc substreams.ResponseFunc)
	Send(respFunc substreams.ResponseFunc) error
}

type bytesMeter struct {
	modules map[string]struct{}

	bytesWrittenMap map[string]uint64
	bytesReadMap    map[string]uint64

	mu     sync.RWMutex
	logger *zap.Logger
}

func NewBytesMeter() BytesMeter {
	return &bytesMeter{
		bytesWrittenMap: map[string]uint64{},
		bytesReadMap:    map[string]uint64{},
		modules:         map[string]struct{}{},
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

	var in []*pbsubstreams.ModuleProgress

	for module := range b.modules {
		written := b.bytesWrittenMap[module]
		read := b.bytesReadMap[module]

		in = append(in, &pbsubstreams.ModuleProgress{
			Name: module,
			Type: &pbsubstreams.ModuleProgress_ProcessedBytes_{
				ProcessedBytes: &pbsubstreams.ModuleProgress_ProcessedBytes{
					TotalBytesWritten: written,
					TotalBytesRead:    read,
				},
			},
		})
	}

	resp := substreams.NewModulesProgressResponse(in)
	err := respFunc(resp)
	if err != nil {
		return err
	}

	return nil
}

func (b *bytesMeter) AddBytesWritten(module string, n int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if module == "" {
		panic("module is empty")
	}
	b.modules[module] = struct{}{}

	if n < 0 {
		panic("negative value")
	}

	b.bytesWrittenMap[module] += uint64(n)
}

func (b *bytesMeter) AddBytesRead(module string, n int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if module == "" {
		panic("module is empty")
	}
	b.modules[module] = struct{}{}

	if n < 0 {
		panic("negative value")
	}

	b.bytesReadMap[module] += uint64(n)
}

func (b *bytesMeter) BytesWritten(module string) uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesWrittenMap[module]
}

func (b *bytesMeter) BytesRead(module string) uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesReadMap[module]
}

type noopBytesMeter struct{}

func (_ *noopBytesMeter) AddBytesWritten(module string, n int)                         { return }
func (_ *noopBytesMeter) AddBytesRead(module string, n int)                            { return }
func (_ *noopBytesMeter) BytesWritten(module string) uint64                            { return 0 }
func (_ *noopBytesMeter) BytesRead(module string) uint64                               { return 0 }
func (_ *noopBytesMeter) Launch(ctx context.Context, respFunc substreams.ResponseFunc) {}
func (_ *noopBytesMeter) Send(respFunc substreams.ResponseFunc) error                  { return nil }

var NoopBytesMeter BytesMeter = &noopBytesMeter{}
