package postgres

import (
	"testing"

	"github.com/streamingfast/substreams/sink/sql/bytes"
	sql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/require"
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
