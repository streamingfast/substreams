package cdc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEvent(id string, op Op, block uint64, table string) Event {
	e := Event{
		ID:  id,
		Op:  op,
		Key: map[string]string{"id": id},
		Source: Source{
			Module:  "db_out",
			Table:   table,
			Block:   Block{Number: block, ID: "b" + id, Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
			Ordinal: 1,
		},
	}
	if op != OpDelete {
		e.After = map[string]any{"id": id}
	}
	return e
}

func TestEvent_EncodeJSON_Golden(t *testing.T) {
	e := Event{
		ID:      "0xabc:transfers:0",
		Op:      OpUpdate,
		Key:     map[string]string{"id": "1"},
		After:   map[string]any{"b": "<2>", "a": "1"},
		Partial: true,
		Source: Source{
			Module: "db_out", Table: "transfers",
			Block:   Block{Number: 10, ID: "0xabc", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
			Ordinal: 3, Final: true, Cursor: "cur",
		},
	}

	got, err := e.EncodeJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"0xabc:transfers:0","op":"u","key":{"id":"1"},"before":null,"after":{"a":"1","b":"<2>"},"partial":true,"source":{"module":"db_out","table":"transfers","block":{"number":10,"id":"0xabc","timestamp":"2026-01-02T03:04:05Z"},"ordinal":3,"final":true,"cursor":"cur"}}`, string(got))
	assert.Equal(t, `{"id":"0xabc:transfers:0","op":"u","key":{"id":"1"},"before":null,"after":{"a":"1","b":"<2>"},"partial":true,"source":{"module":"db_out","table":"transfers","block":{"number":10,"id":"0xabc","timestamp":"2026-01-02T03:04:05Z"},"ordinal":3,"final":true,"cursor":"cur"}}`, string(got), "stable field and key order, no HTML escaping")
}

func TestEvent_EncodeJSON_DeleteAndRevert(t *testing.T) {
	e := newEvent("1", OpDelete, 5, "t")
	e.RevertOf = "1"
	got, err := e.EncodeJSON()
	require.NoError(t, err)
	assert.Contains(t, string(got), `"after":null`)
	assert.Contains(t, string(got), `"revert_of":"1"`)
	assert.NotContains(t, string(got), `"partial"`)

	line, err := e.AppendJSONL(nil)
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), line[len(line)-1])
}
