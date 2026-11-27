// Package cdc offers a Debezium-style change-event output for Substreams data: an event envelope, the
// [Emitter] destinations it is delivered to, a [Journal] of reversible events that turns chain reorgs into
// compensation events, and a [Handler] that plugs all of it into a [sink.Sinker].
package cdc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Op is the kind of change an [Event] carries.
type Op string

const (
	// OpCreate is a new row, `after` holds the full row.
	OpCreate Op = "c"
	// OpUpdate is a changed row, `after` holds only the changed columns (the event is [Event.Partial]).
	OpUpdate Op = "u"
	// OpDelete is a removed row, `after` is null.
	OpDelete Op = "d"
)

// Valid tells whether o is one of the known operations.
func (o Op) Valid() bool {
	return o == OpCreate || o == OpUpdate || o == OpDelete
}

// Block identifies the block an event or batch belongs to.
type Block struct {
	Number    uint64    `json:"number"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp,omitzero"`
}

// Source tells where an [Event] comes from.
type Source struct {
	Module  string `json:"module"`
	Table   string `json:"table"`
	Block   Block  `json:"block"`
	Ordinal uint64 `json:"ordinal"`
	// Final is true when the block was at or below the final block height when the event was produced.
	Final  bool   `json:"final"`
	Cursor string `json:"cursor"`
}

// Event is one change-data-capture envelope, encoded as a single JSON object.
//
// Event sources fill ID (optional, the [Handler] generates one when empty), Op, Key, After, Partial,
// Source.Table and Source.Ordinal. The [Handler] fills the rest of Source.
type Event struct {
	// ID is deterministic and unique within the stream: `<blockId>:<table>:<index>`.
	ID string `json:"id"`
	Op Op     `json:"op"`
	// Key holds the primary key columns.
	Key map[string]string `json:"key"`
	// Before is always null: no before-image is available from Substreams.
	Before map[string]any `json:"before"`
	// After holds the column values, null for a delete. For an update only the changed columns are present.
	After map[string]any `json:"after"`
	// Partial is only set on updates, where After holds only the changed columns.
	Partial bool   `json:"partial,omitempty"`
	Source  Source `json:"source"`
	// RevertOf is only set on compensation events, it is the ID of the event being reverted.
	RevertOf string `json:"revert_of,omitempty"`
}

// EncodeJSON encodes the event as one compact JSON object, without trailing newline and without HTML escaping.
func (e *Event) EncodeJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return nil, fmt.Errorf("encode event %q: %w", e.ID, err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// AppendJSONL appends the event to dst as one JSON line (newline terminated).
func (e *Event) AppendJSONL(dst []byte) ([]byte, error) {
	encoded, err := e.EncodeJSON()
	if err != nil {
		return nil, err
	}
	dst = append(dst, encoded...)
	return append(dst, '\n'), nil
}
