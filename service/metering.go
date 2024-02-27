package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/dmetering"
	"google.golang.org/protobuf/proto"
)

func sendMetering(meter dmetering.Meter, userID, apiKeyID, ip, userMeta, endpoint string, resp proto.Message, logger *zap.Logger) {
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

	// we send metering even if context is canceled
	dmetering.Emit(context.Background(), event)
}
