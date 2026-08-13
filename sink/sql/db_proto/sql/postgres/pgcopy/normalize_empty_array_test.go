package pgcopy

import (
	"bytes"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/stretchr/testify/require"
)

// TestNormalizeEncodesAnEmptyArrayForEveryColumnType covers the row the walker produces for
// a repeated field that happens to be empty, which is every optional list on most blocks.
//
// Binary COPY resolves the encode plan from the Go type, so an empty slice typed from
// anything but the column is refused outright and takes the whole segment with it.
func TestNormalizeEncodesAnEmptyArrayForEveryColumnType(t *testing.T) {
	for name, oid := range map[string]uint32{
		"numeric[]":   pgtype.NumericArrayOID,
		"bigint[]":    pgtype.Int8ArrayOID,
		"int[]":       pgtype.Int4ArrayOID,
		"bytea[]":     pgtype.ByteaArrayOID,
		"text[]":      pgtype.TextArrayOID,
		"varchar[]":   pgtype.VarcharArrayOID,
		"float8[]":    pgtype.Float8ArrayOID,
		"float4[]":    pgtype.Float4ArrayOID,
		"bool[]":      pgtype.BoolArrayOID,
		"timestamp[]": pgtype.TimestampArrayOID,
	} {
		t.Run(name, func(t *testing.T) {
			columns := []Column{{Name: "list", OID: oid}}
			values := []any{[]any{}}

			require.NoError(t, NormalizeRowWithEncoding(columns, values, sqlbytes.EncodingRaw))

			writer, err := NewWriter(&bytes.Buffer{}, columns)
			require.NoError(t, err)
			require.NoError(t, writer.WriteRow(values))
		})
	}
}
