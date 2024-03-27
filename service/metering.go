package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/dmetering"
	"google.golang.org/protobuf/proto"
)

func sendMetering(ctx context.Context, meter dmetering.Meter, userID, apiKeyID, ip, userMeta, endpoint string, resp proto.Message, logger *zap.Logger) {
	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()
	egressBytes := proto.Size(resp)

	inputBytes := meter.GetCount("wasm_input_bytes")
	meter.ResetCount("wasm_input_bytes")

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
			"message_count":    1,
		},
		Timestamp: time.Now(),
	}

	emitter := ctx.Value("event_emitter").(dmetering.EventEmitter)
	if emitter == nil {
		dmetering.Emit(context.WithoutCancel(ctx), event)
	} else {
		emitter.Emit(context.WithoutCancel(ctx), event)
	}
}
