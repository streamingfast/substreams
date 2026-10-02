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
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

const (
	MeterLiveUncompressedReadBytes = "live_uncompressed_read_bytes"

	MeterFileUncompressedReadBytes = "file_uncompressed_read_bytes"
	MeterFileCompressedReadBytes   = "file_compressed_read_bytes"

	MeterFileUncompressedWriteBytes = "file_uncompressed_write_bytes"
	MeterFileCompressedWriteBytes   = "file_compressed_write_bytes"

	MeterUncompressedEgressBytes = "egress_bytes" // named like this to be backwards compatible with the previous metrics
	MeterProcessedBlocks         = "processed_blocks"

	MeterWasmInputBytes = "wasm_input_bytes"

	// MeterExternalCallsPrefix is followed by the kind of external call, ex: "external_calls_eth_call"
	MeterExternalCallsPrefix = "external_calls_"

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

func AddEgressBytes(ctx context.Context, n int) {
	dmetering.GetBytesMeter(ctx).CountInc(MeterUncompressedEgressBytes, n)
}
func AddProcessedBlocks(ctx context.Context, n int) {
	dmetering.GetBytesMeter(ctx).CountInc(MeterProcessedBlocks, n)
}

func AddWasmInputBytes(ctx context.Context, n int) {
	dmetering.GetBytesMeter(ctx).CountInc(MeterWasmInputBytes, n)
}

// externalCallMeters holds the meter name of each kind of external call counted by this process.
var externalCallMeters sync.Map

// AddExternalCalls counts `n` calls of the given kind (ex: "eth_call") made by a wasm extension.
// A batch counts for as many calls as it contains.
func AddExternalCalls(ctx context.Context, kind string, n int) {
	name := MeterExternalCallsPrefix + kind
	externalCallMeters.LoadOrStore(name, struct{}{})
	dmetering.GetBytesMeter(ctx).CountInc(name, n)
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
			var isStepNew bool
			if stepable, ok := obj.(bstream.Stepable); ok {
				step := stepable.Step()
				if step.Matches(bstream.StepNew) || step.Matches(bstream.StepPartial) {
					isStepNew = true
					dmetering.GetBytesMeter(ctx).CountInc(MeterLiveUncompressedReadBytes, len(blk.GetPayload().GetValue()))
				}
			}
			err := next.ProcessBlock(blk, obj)
			if err != nil {
				return err
			}
			if liveable, ok := obj.(bstream.Liveable); ok && isStepNew {
				if liveable.IsLiveBlock() {
					metrics.Tier1OutputHeadBlockRelativeTime.SetLastBlock(blk.Time())
				}
			}

			return nil
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

func (ms *MetricsSender) Send(ctx context.Context, organizationID, apiKeyID, ip, userMeta, outputModuleHash, endpoint string) {
	ms.Lock()
	defer ms.Unlock()

	if reqctx.IsBackfillerRequest(ctx) {
		endpoint = fmt.Sprintf("%s%s", endpoint, "Backfill")
	}

	meter := dmetering.GetBytesMeter(ctx)

	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()
	egressBytes := meter.GetCountAndReset(MeterUncompressedEgressBytes)
	processedBlocks := meter.GetCountAndReset(MeterProcessedBlocks)

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

	eventMetrics := map[string]float64{
		MeterUncompressedEgressBytes:    float64(egressBytes),
		"written_bytes":                 float64(bytesWritten),
		"read_bytes":                    float64(bytesRead),
		MeterWasmInputBytes:             float64(inputBytes),
		MeterLiveUncompressedReadBytes:  float64(liveUncompressedReadBytes),
		MeterFileUncompressedReadBytes:  float64(fileUncompressedReadBytes),
		MeterFileCompressedReadBytes:    float64(fileCompressedReadBytes),
		MeterFileUncompressedWriteBytes: float64(fileUncompressedWriteBytes),
		MeterFileCompressedWriteBytes:   float64(fileCompressedWriteBytes),
		MeterProcessedBlocks:            float64(processedBlocks),
		"message_count":                 1,
	}

	externalCallMeters.Range(func(name, _ any) bool {
		if count := meter.GetCountAndReset(name.(string)); count != 0 {
			eventMetrics[name.(string)] = float64(count)
		}
		return true
	})

	event := dmetering.Event{
		OrganizationID:   organizationID,
		ApiKeyID:         apiKeyID,
		IpAddress:        ip,
		Meta:             userMeta,
		OutputModuleHash: outputModuleHash,

		Endpoint:  endpoint,
		Metrics:   eventMetrics,
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
