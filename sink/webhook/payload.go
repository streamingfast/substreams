package webhook

import (
	"encoding/json"
	"strings"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// Clock represents the clock information in the webhook payload
type Clock struct {
	Timestamp string `json:"timestamp"`
	Number    uint64 `json:"number"`
	ID        string `json:"id"`
}

// Manifest represents the manifest information in the webhook payload
type Manifest struct {
	ModuleName string `json:"moduleName"`
	Type       string `json:"type"`
}

// WebhookPayload represents the payload structure sent to webhook endpoints
type WebhookPayload struct {
	Clock    Clock           `json:"clock"`
	Manifest Manifest        `json:"manifest"`
	Data     json.RawMessage `json:"data"`
}

// NewWebhookPayload creates a new webhook payload with the desired format
func NewWebhookPayload(moduleName string, clock *pbsubstreams.Clock, msgType string, data json.RawMessage) (*WebhookPayload, error) {
	var timestampStr string
	if clock != nil && clock.Timestamp != nil {
		timestampStr = clock.Timestamp.AsTime().Format(time.RFC3339)
	}

	// Strip the "type.googleapis.com/" prefix from the type
	cleanType := msgType
	if strings.HasPrefix(msgType, "type.googleapis.com/") {
		cleanType = strings.TrimPrefix(msgType, "type.googleapis.com/")
	}

	clockInfo := Clock{
		Timestamp: timestampStr,
		Number:    0,
		ID:        "",
	}

	if clock != nil {
		clockInfo.Number = clock.Number
		clockInfo.ID = clock.Id
	}

	manifest := Manifest{
		ModuleName: moduleName,
		Type:       cleanType,
	}

	return &WebhookPayload{
		Clock:    clockInfo,
		Manifest: manifest,
		Data:     data,
	}, nil
}

// ToJSON serializes the webhook payload to JSON bytes
func (p *WebhookPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// BlockRef identifies a block in an undo payload.
type BlockRef struct {
	Number uint64 `json:"number"`
	ID     string `json:"id"`
}

// UndoManifest names the module whose blocks are being undone.
type UndoManifest struct {
	ModuleName string `json:"moduleName"`
}

// UndoPayload is sent to the undo URL when the chain reorganizes. Every block
// the receiver got with a number above LastValidBlock is no longer on the
// chain; the blocks that replace them follow as regular payloads.
type UndoPayload struct {
	LastValidBlock BlockRef     `json:"lastValidBlock"`
	Manifest       UndoManifest `json:"manifest"`
}

// NewUndoPayload creates the payload for an undo signal.
func NewUndoPayload(moduleName string, lastValidBlock *pbsubstreams.BlockRef) *UndoPayload {
	p := &UndoPayload{Manifest: UndoManifest{ModuleName: moduleName}}
	if lastValidBlock != nil {
		p.LastValidBlock = BlockRef{Number: lastValidBlock.Number, ID: lastValidBlock.Id}
	}
	return p
}

// ToJSON serializes the undo payload to JSON bytes
func (p *UndoPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}
