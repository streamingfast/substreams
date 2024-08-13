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

	liveBytes := meter.GetCount("live_bytes_read")
	if liveBytes > 0 {
		panic("live_bytes should be 0 at this point")
	}
	meter.ResetCount("live_bytes_read")

	event := dmetering.Event{
		UserID:    userID,
		ApiKeyID:  apiKeyID,
		IpAddress: ip,
		Meta:      userMeta,

		Endpoint: endpoint,
		Metrics: map[string]float64{
			"egress_bytes":     float64(egressBytes),
			"written_bytes":    float64(bytesWritten),
			"read_bytes":       float64(bytesRead),
			"wasm_input_bytes": float64(inputBytes),
			"live_bytes":       float64(liveBytes),
			"message_count":    1,
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
