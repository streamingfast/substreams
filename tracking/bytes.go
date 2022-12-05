package tracking

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type BytesMeter interface {
	AddBytesWritten(n int)
	AddBytesRead(n int)

	BytesWritten() uint64
	BytesRead() uint64

	BytesWrittenDelta() uint64
	BytesReadDelta() uint64

	Launch(ctx context.Context, respFunc substreams.ResponseFunc)
	Send(ctx context.Context, respFunc substreams.ResponseFunc) error
}

type bytesMeter struct {
	bytesWritten uint64
	bytesRead    uint64

	bytesWrittenDelta uint64
	bytesReadDelta    uint64

	lastTime time.Time

	mu     sync.RWMutex
	logger *zap.Logger
}

func NewBytesMeter(ctx context.Context) BytesMeter {
	return &bytesMeter{
		logger:   reqctx.Logger(ctx),
		lastTime: time.Now(),
	}
}

func (b *bytesMeter) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return fmt.Sprintf("bytes written: %d, bytes read: %d", b.bytesWritten, b.bytesRead)
}

func (b *bytesMeter) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	enc.AddUint64("total_bytes_written", b.bytesWritten)
	enc.AddUint64("bytes_written_delta", b.bytesWrittenDelta)
	enc.AddUint64("total_bytes_read", b.bytesRead)
	enc.AddUint64("bytes_read_delta", b.bytesReadDelta)
	enc.AddDuration("time_delta", time.Since(b.lastTime))
	return nil
}

func (b *bytesMeter) Start(ctx context.Context, respFunc substreams.ResponseFunc) {
	logger := reqctx.Logger(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			err := b.Send(ctx, respFunc)
			if err != nil {
				logger.Error("unable to send bytes meter", zap.Error(err))
			}
		}
	}
}

func (b *bytesMeter) Launch(ctx context.Context, respFunc substreams.ResponseFunc) {
	go b.Start(ctx, respFunc)
}

func (b *bytesMeter) resetDeltas() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.bytesWrittenDelta = 0
	b.bytesReadDelta = 0
	b.lastTime = time.Now()
}

func (b *bytesMeter) Send(ctx context.Context, respFunc substreams.ResponseFunc) error {
	defer func() {
		stats := reqctx.ReqStats(ctx)
		stats.RecordBytesWritten(b.BytesWrittenDelta())
		stats.RecordBytesRead(b.BytesReadDelta())
	}()

	b.mu.RLock()
	defer b.mu.RUnlock()

	var in []*pbsubstreams.ModuleProgress

	in = append(in, &pbsubstreams.ModuleProgress{
		Name: "",
		Type: &pbsubstreams.ModuleProgress_ProcessedBytes_{
			ProcessedBytes: &pbsubstreams.ModuleProgress_ProcessedBytes{
				TotalBytesWritten: b.bytesWritten,
				TotalBytesRead:    b.bytesRead,
				BytesWrittenDelta: b.bytesWrittenDelta,
				BytesReadDelta:    b.bytesReadDelta,
				NanoSecondsDelta:  uint64(time.Since(b.lastTime).Nanoseconds()),
			},
		},
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

	b.bytesWrittenDelta += uint64(n)
	b.bytesWritten += uint64(n)
}

func (b *bytesMeter) AddBytesRead(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.bytesReadDelta += uint64(n)
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

func (b *bytesMeter) BytesWrittenDelta() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesWrittenDelta
}

func (b *bytesMeter) BytesReadDelta() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesReadDelta
}

type noopBytesMeter struct{}

func (_ *noopBytesMeter) AddBytesWritten(n int)                                        { return }
func (_ *noopBytesMeter) AddBytesRead(n int)                                           { return }
func (_ *noopBytesMeter) BytesWritten() uint64                                         { return 0 }
func (_ *noopBytesMeter) BytesRead() uint64                                            { return 0 }
func (_ *noopBytesMeter) BytesWrittenDelta() uint64                                    { return 0 }
func (_ *noopBytesMeter) BytesReadDelta() uint64                                       { return 0 }
func (_ *noopBytesMeter) Launch(ctx context.Context, respFunc substreams.ResponseFunc) {}
func (_ *noopBytesMeter) Send(ctx context.Context, respFunc substreams.ResponseFunc) error {
	return nil
}

var NoopBytesMeter BytesMeter = &noopBytesMeter{}
