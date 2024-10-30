package metering

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	MeterLiveUncompressedReadBytes = "live_uncompressed_read_bytes"

	MeterFileUncompressedReadBytes = "file_uncompressed_read_bytes"
	MeterFileCompressedReadBytes   = "file_compressed_read_bytes"

	MeterFileUncompressedWriteBytes = "file_uncompressed_write_bytes"
	MeterFileCompressedWriteBytes   = "file_compressed_write_bytes"

	MeterWasmInputBytes = "wasm_input_bytes"

	TotalReadBytes  = "total_read_bytes"
	TotalWriteBytes = "total_write_bytes"
)

func WithBlockBytesReadMeteringOptions(meter dmetering.Meter, logger *zap.Logger) []dstore.Option {
	var opts []dstore.Option
	opts = append(opts, dstore.WithCompressedReadCallback(func(ctx context.Context, n int) {
		meter.CountInc(MeterFileCompressedReadBytes, n)
	}))

	// uncompressed read bytes is measured in the file source middleware.

	// no writes are done to this store, so no need to measure write bytes

	return opts
}

func WithBytesMeteringOptions(meter dmetering.Meter, logger *zap.Logger) []dstore.Option {
	var opts []dstore.Option
	opts = append(opts, dstore.WithUncompressedReadCallback(func(ctx context.Context, n int) {
		meter.CountInc(MeterFileUncompressedReadBytes, n)
	}))
	opts = append(opts, dstore.WithCompressedReadCallback(func(ctx context.Context, n int) {
		meter.CountInc(MeterFileCompressedReadBytes, n)
	}))
	opts = append(opts, dstore.WithUncompressedWriteCallback(func(ctx context.Context, n int) {
		meter.CountInc(MeterFileUncompressedWriteBytes, n)
	}))
	opts = append(opts, dstore.WithCompressedWriteCallback(func(ctx context.Context, n int) {
		meter.CountInc(MeterFileCompressedWriteBytes, n)
	}))

	return opts
}

func AddWasmInputBytes(ctx context.Context, n int) {
	dmetering.GetBytesMeter(ctx).CountInc(MeterWasmInputBytes, n)
}

func GetTotalBytesRead(meter dmetering.Meter) uint64 {
	total := uint64(meter.GetCount(TotalReadBytes))
	return total
}

func GetTotalBytesWritten(meter dmetering.Meter) uint64 {
	total := uint64(meter.GetCount(TotalWriteBytes))
	return total
}

func LiveSourceMiddlewareHandlerFactory(ctx context.Context) func(handler bstream.Handler) bstream.Handler {
	return func(next bstream.Handler) bstream.Handler {
		return bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
			if stepable, ok := obj.(bstream.Stepable); ok {
				step := stepable.Step()
				if step.Matches(bstream.StepNew) {
					dmetering.GetBytesMeter(ctx).CountInc(MeterLiveUncompressedReadBytes, len(blk.GetPayload().GetValue()))
				}
			}
			return next.ProcessBlock(blk, obj)
		})
	}
}

func FileSourceMiddlewareHandlerFactory(ctx context.Context) func(handler bstream.Handler) bstream.Handler {
	return func(next bstream.Handler) bstream.Handler {
		return bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
			if stepable, ok := obj.(bstream.Stepable); ok {
				step := stepable.Step()
				if step.Matches(bstream.StepNew) {
					dmetering.GetBytesMeter(ctx).CountInc(MeterFileUncompressedReadBytes, len(blk.GetPayload().GetValue()))
				}
			}
			return next.ProcessBlock(blk, obj)
		})
	}
}

type MetricsSender struct {
	sync.Mutex
}

func NewMetricsSender() *MetricsSender {
	return &MetricsSender{
		Mutex: sync.Mutex{},
	}
}

func (ms *MetricsSender) Send(ctx context.Context, userID, apiKeyID, ip, userMeta, outputModuleHash, endpoint string, resp proto.Message) {
	ms.Lock()
	defer ms.Unlock()

	if reqctx.IsBackfillerRequest(ctx) {
		endpoint = fmt.Sprintf("%s%s", endpoint, "Backfill")
	}

	meter := dmetering.GetBytesMeter(ctx)

	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()
	egressBytes := proto.Size(resp)

	inputBytes := meter.GetCountAndReset(MeterWasmInputBytes)

	liveUncompressedReadBytes := meter.GetCountAndReset(MeterLiveUncompressedReadBytes)
	fileUncompressedReadBytes := meter.GetCountAndReset(MeterFileUncompressedReadBytes)
	fileCompressedReadBytes := meter.GetCountAndReset(MeterFileCompressedReadBytes)

	fileUncompressedWriteBytes := meter.GetCountAndReset(MeterFileUncompressedWriteBytes)
	fileCompressedWriteBytes := meter.GetCountAndReset(MeterFileCompressedWriteBytes)

	totalReadBytes := fileUncompressedReadBytes + liveUncompressedReadBytes
	totalWriteBytes := fileUncompressedWriteBytes

	meter.CountInc(TotalReadBytes, int(totalReadBytes))
	meter.CountInc(TotalWriteBytes, int(totalWriteBytes))

	event := dmetering.Event{
		UserID:           userID,
		ApiKeyID:         apiKeyID,
		IpAddress:        ip,
		Meta:             userMeta,
		OutputModuleHash: outputModuleHash,

		Endpoint: endpoint,
		Metrics: map[string]float64{
			"egress_bytes":                  float64(egressBytes),
			"written_bytes":                 float64(bytesWritten),
			"read_bytes":                    float64(bytesRead),
			MeterWasmInputBytes:             float64(inputBytes),
			MeterLiveUncompressedReadBytes:  float64(liveUncompressedReadBytes),
			MeterFileUncompressedReadBytes:  float64(fileUncompressedReadBytes),
			MeterFileCompressedReadBytes:    float64(fileCompressedReadBytes),
			MeterFileUncompressedWriteBytes: float64(fileUncompressedWriteBytes),
			MeterFileCompressedWriteBytes:   float64(fileCompressedWriteBytes),
			"message_count":                 1,
		},
		Timestamp: time.Now(),
	}

	emitter := reqctx.Emitter(ctx)
	if emitter == nil {
		dmetering.Emit(context.WithoutCancel(ctx), event)
	} else {
		emitter.Emit(context.WithoutCancel(ctx), event)
	}
}

func WithMetricsSender(ctx context.Context) context.Context {
	if _, ok := ctx.Value("metrics_sender").(*MetricsSender); ok {
		return ctx
	}

	sender := NewMetricsSender()
	return context.WithValue(ctx, "metrics_sender", sender)
}

func GetMetricsSender(ctx context.Context) *MetricsSender {
	sender, ok := ctx.Value("metrics_sender").(*MetricsSender)
	if !ok {
		panic("metrics sender not set")
	}
	return sender
}
