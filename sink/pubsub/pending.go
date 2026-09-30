package pubsub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// pendingDelivery is a payload that could not be published, kept on disk next
// to the cursor file. It is written when every attempt has failed in
// OnFailureExit mode and removed once the payload goes through, so the next
// start publishes it before a Substreams stream is opened. A kill in the
// middle of a publish leaves no file: the cursor was not advanced, so the
// stream re-sends that block. The record must stay in sync with the webhook
// sink's pending delivery (sink/webhook).
type pendingDelivery struct {
	// Kind selects a block payload from a reorg notification. Empty reads as
	// DeliveryKindBlock.
	Kind DeliveryKind `json:"kind,omitempty"`
	// Batched marks a block payload in the BatchPayload shape. A pending
	// payload whose shape does not match the mode the sink now runs in is
	// discarded on start and its blocks come back through the stream.
	Batched     bool            `json:"batched,omitempty"`
	Cursor      string          `json:"cursor"`
	BlockNumber uint64          `json:"block_number"`
	Payload     json.RawMessage `json:"payload"`
	// FirstAttemptAt is set when the file is created and is never moved
	// forward by a retry. It is the start of the current outage.
	FirstAttemptAt time.Time `json:"first_attempt_at"`
	// Fingerprint identifies the topic and undo setting in effect when the
	// file was created. A different fingerprint on restart means the
	// configuration changed, which resets FirstAttemptAt.
	Fingerprint string `json:"fingerprint"`
}

// DeliveryKind tells a block payload from a reorg notification.
type DeliveryKind string

const (
	DeliveryKindBlock DeliveryKind = "block"
	DeliveryKindUndo  DeliveryKind = "undo"
)

func (p *pendingDelivery) isUndo() bool { return p.Kind == DeliveryKindUndo }

func (p *pendingDelivery) messageType() string {
	if p.isUndo() {
		return TypeUndo
	}
	if p.Batched {
		return TypeBatch
	}
	return TypeBlock
}

// pendingFilePath derives the pending file from the state file. Both must
// live on the same persistent volume, so one setting places the two.
func pendingFilePath(stateFile string) string {
	if stateFile == "" {
		return ""
	}
	return stateFile + ".pending"
}

// configFingerprint hashes what a retry of a pending payload depends on.
func configFingerprint(project, topic string, undo bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%t", project, topic, undo)
	return hex.EncodeToString(h.Sum(nil))
}

// readPending returns nil, nil when there is no pending file.
func readPending(path string) (*pendingDelivery, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var p pendingDelivery
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decoding pending file %q: %w", path, err)
	}
	return &p, nil
}

// writePending writes the file through a temp file and a rename so a reader
// never sees a partial file.
func writePending(path string, p *pendingDelivery) error {
	if path == "" {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".pending_*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func removePending(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
