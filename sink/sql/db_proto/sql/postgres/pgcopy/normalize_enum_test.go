package pgcopy

import (
	"bytes"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/require"
)

// TestNormalizeEncodesAnEnumArray covers a repeated enum field, which the walk hands over
// as EnumValue and MapFieldType declares TEXT[]. It has to reach the server as the name,
// which is what the rendered write modes write for the same value.
func TestNormalizeEncodesAnEnumArray(t *testing.T) {
	columns := []Column{{Name: "kinds", OID: pgtype.TextArrayOID}}
	values := []any{[]any{
		sql2.EnumValue{Number: 2, Name: "ACTIVE"},
		sql2.EnumValue{Number: 3, Name: "CLOSED"},
	}}

	require.NoError(t, NormalizeRowWithEncoding(columns, values, sqlbytes.EncodingRaw))

	var buf bytes.Buffer
	writer, err := NewWriter(&buf, columns)
	require.NoError(t, err)
	require.NoError(t, writer.WriteRow(values))
	require.NoError(t, writer.Close())

	require.Contains(t, buf.String(), "ACTIVE")
	require.Contains(t, buf.String(), "CLOSED")
}
