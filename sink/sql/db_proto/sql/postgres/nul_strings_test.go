package postgres

import (
	"testing"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestJSONWithoutNul(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		expected string
		changed  bool
	}{
		{name: "no escape", in: `{"a":"b"}`, expected: `{"a":"b"}`},
		{name: "escaped NUL", in: `{"a":"x\u0000y"}`, expected: `{"a":"xy"}`, changed: true},
		{name: "only NUL", in: `{"a":"\u0000\u0000"}`, expected: `{"a":""}`, changed: true},
		{name: "escaped backslash then u0000 is text", in: `{"a":"\\u0000"}`, expected: `{"a":"\\u0000"}`},
		{name: "both", in: `{"a":"\\u0000\u0000"}`, expected: `{"a":"\\u0000"}`, changed: true},
		{name: "other escapes are kept", in: `{"a":"\"\n\u0001"}`, expected: `{"a":"\"\n\u0001"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed := jsonWithoutNul(c.in)
			require.Equal(t, c.expected, out)
			require.Equal(t, c.changed, changed)
		})
	}
}

func TestRemoveNulCharactersLogsEachColumnOnce(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	d := &Database{
		logger: zap.New(core),
		rowLayouts: map[string][]rowValue{"t": {
			{name: "_block_number_"},
			{name: "_block_timestamp_"},
			{name: "name", column: &schema.Column{Name: "name"}},
			{name: "doc", column: &schema.Column{Name: "doc", Nested: &schema.Table{}}},
			{name: "tags", column: &schema.Column{Name: "tags"}},
		}},
		nulColumnsLogged: map[string]struct{}{},
	}

	row := []any{uint64(1), nil, "a\x00", `{"x":"\u0000"}`, []any{"b\x00", "c"}}
	d.removeNulCharacters("t", row)
	require.Equal(t, []any{uint64(1), nil, "a", `{"x":""}`, []any{"b", "c"}}, row)

	d.removeNulCharacters("t", []any{uint64(2), nil, "\x00", "", []any{}})

	var columns []string
	for _, entry := range logs.All() {
		columns = append(columns, entry.ContextMap()["column"].(string))
	}
	require.Equal(t, []string{"name", "doc", "tags"}, columns)
}
