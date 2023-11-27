package service

import (
	"context"
	"time"

	"github.com/streamingfast/dmetering"
	"google.golang.org/protobuf/proto"
)

func sendMetering(meter dmetering.Meter, userID, apiKeyID, ip, userMeta, endpoint string, resp proto.Message) {
	bytesRead := meter.BytesReadDelta()
	bytesWritten := meter.BytesWrittenDelta()

	event := dmetering.Event{
		UserID:    userID,
		ApiKeyID:  apiKeyID,
		IpAddress: ip,
		Meta:      userMeta,

		Endpoint: endpoint,
		Metrics: map[string]float64{
			"egress_bytes":  float64(proto.Size(resp)),
			"written_bytes": float64(bytesWritten),
			"read_bytes":    float64(bytesRead),
			"message_count": 1,
		},
		Timestamp: time.Now(),
	}

	// we send metering even if context is canceled
	dmetering.Emit(context.Background(), event)
}
