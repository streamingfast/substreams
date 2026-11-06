package postgres

import (
	"math"
	"testing"
	"time"

	"github.com/streamingfast/substreams/sink/sql/bytes"
	sql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestValueToStringStoresWhatCopyWouldStore pins the rendered write modes against the
// binary COPY one. --write-mode picks between them for throughput, so a value that
// survives one path and not the other makes the same substream produce different rows
// depending on a performance flag.
//
// The literals below are what the server stores under standard_conforming_strings = on,
// the default since PostgreSQL 9.1, which is also what binary COPY writes verbatim.
func TestValueToStringStoresWhatCopyWouldStore(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		expected string
	}{
		{name: "plain", in: "hello", expected: "'hello'"},
		{name: "a quote is doubled", in: "it's", expected: "'it''s'"},
		{name: "a backslash is stored as one", in: `C:\path`, expected: `'C:\path'`},
		{name: "an escape sequence is not one", in: `a\nb`, expected: `'a\nb'`},
		{name: "both at once", in: `a\'b`, expected: `'a\''b'`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, ValueToString(c.in, bytes.EncodingRaw))
		})
	}
}

func TestValueToStringRendersAnEnumByName(t *testing.T) {
	require.Equal(t, "'TRANSFER'", ValueToString(sql.EnumValue{Number: 1, Name: "TRANSFER"}, bytes.EncodingRaw))
	require.Equal(t, "'7'", ValueToString(sql.EnumValue{Number: 7}, bytes.EncodingRaw))
}

func TestValueToStringRendersATextArray(t *testing.T) {
	require.Equal(t, `'{"{\"a\":\"it''s\"}","{}"}'`, ValueToString(postgresTextArray{`{"a":"it's"}`, `{}`}, bytes.EncodingRaw))
	require.Equal(t, "'{}'", ValueToString(postgresTextArray{}, bytes.EncodingRaw))
	require.Equal(t, "NULL", ValueToString(postgresTextArray(nil), bytes.EncodingRaw))
}

// TestValueToStringRendersWhatCopyStores covers values whose rendered literal PostgreSQL
// used to reject, or read as something other than what binary COPY writes.
func TestValueToStringRendersWhatCopyStores(t *testing.T) {
	cases := []struct {
		name     string
		in       any
		expected string
	}{
		{name: "nil", in: nil, expected: "NULL"},
		{name: "NaN", in: math.NaN(), expected: "'NaN'"},
		{name: "infinity", in: math.Inf(1), expected: "'Infinity'"},
		{name: "float32 negative infinity", in: float32(math.Inf(-1)), expected: "'-Infinity'"},
		{name: "finite float", in: 1.5, expected: "1.5"},
		{name: "time keeps microseconds", in: time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC), expected: "'2025-01-02 03:04:05.123456'"},
		{name: "time in UTC", in: time.Date(2025, 1, 2, 3, 4, 5, 0, time.FixedZone("", 3600)), expected: "'2025-01-02 02:04:05'"},
		{name: "timestamp keeps microseconds", in: timestamppb.New(time.Date(2025, 1, 2, 3, 4, 5, 1000, time.UTC)), expected: "'2025-01-02 03:04:05.000001'"},
		{name: "array is an untyped literal", in: []any{"1", "it's"}, expected: `'{"1","it''s"}'`},
		{name: "array of floats", in: []any{math.NaN(), math.Inf(-1), 0.5}, expected: `'{"NaN","-Infinity","0.5"}'`},
		{name: "empty array", in: []any{}, expected: "'{}'"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, ValueToString(c.in, bytes.EncodingRaw))
		})
	}
}
