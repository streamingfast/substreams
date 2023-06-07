package service

import (
	"context"
	"github.com/streamingfast/dmetering"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"time"
)

func sendMetering(ctx context.Context, logger *zap.Logger, endpoint string, resp proto.Message) {
	meter := dmetering.GetBytesMeter(ctx)
	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()

	userId, apiKeyId, ip := getAuthDetails(ctx)

	event := dmetering.Event{
		UserID:    userId,
		ApiKeyID:  apiKeyId,
		IpAddress: ip,

		Endpoint: endpoint,
		Metrics: map[string]float64{
			"egress_bytes":  float64(proto.Size(resp)),
			"written_bytes": float64(bytesWritten),
			"read_bytes":    float64(bytesRead),
		},
		Timestamp: time.Now(),
	}

	dmetering.Emit(ctx, event)
}
