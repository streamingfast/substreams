package service

import (
	"context"
	"time"

	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/substreams/reqctx"
	"google.golang.org/protobuf/proto"
)

func sendMetering(ctx context.Context, meter dmetering.Meter, userID, apiKeyID, ip, userMeta, endpoint string, resp proto.Message) {
	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()
	egressBytes := proto.Size(resp)

	inputBytes := meter.GetCount("wasm_input_bytes")
	meter.ResetCount("wasm_input_bytes")

	uncompressedBytesRead := meter.GetCount("uncompressed_bytes_read")
	meter.ResetCount("uncompressed_bytes_read")

	uncompressedLiveBytesRead := meter.GetCount("uncompressed_live_bytes_read")
	meter.ResetCount("uncompressed_live_bytes_read")

	event := dmetering.Event{
		UserID:    userID,
		ApiKeyID:  apiKeyID,
		IpAddress: ip,
		Meta:      userMeta,

		Endpoint: endpoint,
		Metrics: map[string]float64{
			"egress_bytes":                 float64(egressBytes),
			"written_bytes":                float64(bytesWritten),
			"read_bytes":                   float64(bytesRead),
			"wasm_input_bytes":             float64(inputBytes),
			"uncompressed_bytes_read":      float64(uncompressedBytesRead),
			"uncompressed_live_bytes_read": float64(uncompressedLiveBytesRead),
			"message_count":                1,
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
