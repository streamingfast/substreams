package webhook

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// WebhookPayload represents the payload structure sent to webhook endpoints
type WebhookPayload struct {
	Module    string          `json:"module"`
	Block     uint64          `json:"block"`
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// NewWebhookPayload creates a new webhook payload with the desired format
func NewWebhookPayload(moduleName string, blockNum uint64, timestamp *timestamppb.Timestamp, msgType string, data json.RawMessage) (*WebhookPayload, error) {
	var timestampStr string
	if timestamp != nil {
		timestampStr = timestamp.AsTime().Format(time.RFC3339)
	}

	return &WebhookPayload{
		Module:    moduleName,
		Block:     blockNum,
		Timestamp: timestampStr,
		Type:      msgType,
		Payload:   data,
	}, nil
}

// ToJSON serializes the webhook payload to JSON bytes
func (p *WebhookPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}
