package webhook

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

// pendingDelivery is a payload that could not be delivered, kept on disk next
// to the cursor file. It is written when every attempt has failed in
// OnFailureExit mode and removed once the payload goes through, so the next
// start delivers it before a Substreams stream is opened. A kill in the
// middle of a call leaves no file: the cursor was not advanced, so the stream
// re-sends that block.
type pendingDelivery struct {
	// Kind is pendingKindBlock or pendingKindUndo and selects the URL the
	// payload goes to. Empty reads as pendingKindBlock.
	Kind string `json:"kind,omitempty"`
	// Batched marks a block payload in the BatchPayload shape. A pending
	// payload whose shape does not match the mode the sink now runs in is
	// discarded on start and its blocks come back through the stream.
	Batched     bool   `json:"batched,omitempty"`
	Cursor      string `json:"cursor"`
	BlockNumber uint64 `json:"block_number"`
	// Payload holds the exact bytes that were attempted, so a retry is
	// byte-identical and a signature computed over it still verifies.
	Payload json.RawMessage `json:"payload"`
	// FirstAttemptAt is set when the file is created and is never moved
	// forward by a retry. It is the start of the current outage.
	FirstAttemptAt time.Time `json:"first_attempt_at"`
	// Fingerprint identifies the delivery configuration (URL and secrets) in
	// effect when the file was created. A different fingerprint on restart
	// means the user changed the configuration, which resets FirstAttemptAt.
	Fingerprint string `json:"fingerprint"`
}

const (
	pendingKindBlock = "block"
	pendingKindUndo  = "undo"
)

func (p *pendingDelivery) isUndo() bool { return p.Kind == pendingKindUndo }

// pendingFilePath derives the pending file from the state file. Both must
// live on the same persistent volume, so one setting places the two.
func pendingFilePath(stateFile string) string {
	if stateFile == "" {
		return ""
	}
	return stateFile + ".pending"
}

// configFingerprint hashes what the delivery depends on. The secrets are
// hashed rather than stored so the pending file never holds them.
func configFingerprint(url, undoURL string, cfg Config) string {
	h := sha256.New()
	for _, part := range []string{url, undoURL, cfg.AuthHeaderName, cfg.AuthHeaderValue, cfg.SigningSecret} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
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
