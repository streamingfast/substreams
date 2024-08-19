package metering

import (
	"context"
	"time"

	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	MeterLiveUncompressedReadBytes       = "live_uncompressed_read_bytes"
	MeterLiveUncompressedReadForkedBytes = "live_uncompressed_read_forked_bytes"

	MeterFileUncompressedReadBytes = "file_uncompressed_read_bytes"
	MeterFileCompressedReadBytes   = "file_compressed_read_bytes"

	MeterFileUncompressedReadForkedBytes = "file_uncompressed_read_forked_bytes"
	MeterFileCompressedReadForkedBytes   = "file_compressed_read_forked_bytes"

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

	return opts
}

func WithForkedBlockBytesReadMeteringOptions(meter dmetering.Meter, logger *zap.Logger) []dstore.Option {
	var opts []dstore.Option
	opts = append(opts, dstore.WithCompressedReadCallback(func(ctx context.Context, n int) {
		meter.CountInc(MeterFileCompressedReadForkedBytes, n)
	}))

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

func GetTotalBytesRead(meter dmetering.Meter) uint64 {
	total := uint64(meter.GetCount(TotalReadBytes))
	return total
}

func GetTotalBytesWritten(meter dmetering.Meter) uint64 {
	total := uint64(meter.GetCount(TotalWriteBytes))
	return total
}

func Send(ctx context.Context, meter dmetering.Meter, userID, apiKeyID, ip, userMeta, endpoint string, resp proto.Message) {
	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()
	egressBytes := proto.Size(resp)

	inputBytes := meter.GetCountAndReset(MeterWasmInputBytes)

	liveUncompressedReadBytes := meter.GetCountAndReset(MeterLiveUncompressedReadBytes)
	liveUncompressedReadForkedBytes := meter.GetCountAndReset(MeterLiveUncompressedReadForkedBytes)
	fileUncompressedReadBytes := meter.GetCountAndReset(MeterFileUncompressedReadBytes)
	fileUncompressedReadForkedBytes := meter.GetCountAndReset(MeterFileUncompressedReadForkedBytes)
	fileCompressedReadForkedBytes := meter.GetCountAndReset(MeterFileCompressedReadForkedBytes)
	fileCompressedReadBytes := meter.GetCountAndReset(MeterFileCompressedReadBytes)

	fileUncompressedWriteBytes := meter.GetCountAndReset(MeterFileUncompressedWriteBytes)
	fileCompressedWriteBytes := meter.GetCountAndReset(MeterFileCompressedWriteBytes)

	totalReadBytes := fileCompressedReadBytes + fileCompressedReadForkedBytes + liveUncompressedReadBytes + liveUncompressedReadForkedBytes
	totalWriteBytes := fileUncompressedWriteBytes

	meter.CountInc(TotalReadBytes, int(totalReadBytes))
	meter.CountInc(TotalWriteBytes, int(totalWriteBytes))

	event := dmetering.Event{
		UserID:    userID,
		ApiKeyID:  apiKeyID,
		IpAddress: ip,
		Meta:      userMeta,

		Endpoint: endpoint,
		Metrics: map[string]float64{
			"egress_bytes":                       float64(egressBytes),
			"written_bytes":                      float64(bytesWritten),
			"read_bytes":                         float64(bytesRead),
			MeterWasmInputBytes:                  float64(inputBytes),
			MeterLiveUncompressedReadBytes:       float64(liveUncompressedReadBytes),
			MeterLiveUncompressedReadForkedBytes: float64(liveUncompressedReadForkedBytes),
			MeterFileUncompressedReadBytes:       float64(fileUncompressedReadBytes),
			MeterFileUncompressedReadForkedBytes: float64(fileUncompressedReadForkedBytes),
			MeterFileCompressedReadForkedBytes:   float64(fileCompressedReadForkedBytes),
			MeterFileCompressedReadBytes:         float64(fileCompressedReadBytes),
			MeterFileUncompressedWriteBytes:      float64(fileUncompressedWriteBytes),
			MeterFileCompressedWriteBytes:        float64(fileCompressedWriteBytes),
			"message_count":                      1,
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
