package pgcopy

import (
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sqltypes "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/require"
)

func TestNormalizeNumericStringArrayForCopy(t *testing.T) {
	values := []any{[]any{
		"123456789012345678901234567890",
		"",
	}}
	columns := []Column{{Name: "amounts", OID: pgtype.NumericArrayOID}}

	require.NoError(t, NormalizeRow(columns, values))

	expectedLarge, err := numericFromString("123456789012345678901234567890")
	require.NoError(t, err)
	expectedZero, err := numericFromString("0")
	require.NoError(t, err)

	normalized, ok := values[0].([]any)
	require.True(t, ok, "expected numeric array elements, got %T", values[0])
	require.Equal(t, []any{expectedLarge, expectedZero}, normalized)
}

// TestNormalizeEmptyArrayEncodesForEveryArrayType pins the empty-array shape against the
// real encoder rather than against an expected Go type: what matters is that pgx finds a
// binary plan for it, which is exactly what a wrongly typed empty slice fails to do.
func TestNormalizeEmptyArrayEncodesForEveryArrayType(t *testing.T) {
	// Every array type the from-proto dialect can declare for a repeated field.
	oids := []uint32{
		pgtype.NumericArrayOID,
		pgtype.Int8ArrayOID,
		pgtype.Int4ArrayOID,
		pgtype.Int2ArrayOID,
		pgtype.Float8ArrayOID,
		pgtype.Float4ArrayOID,
		pgtype.BoolArrayOID,
		pgtype.ByteaArrayOID,
		pgtype.TextArrayOID,
		pgtype.VarcharArrayOID,
		pgtype.TimestampArrayOID,
		pgtype.TimestamptzArrayOID,
		pgtype.DateArrayOID,
		pgtype.JSONBArrayOID,
	}

	for _, oid := range oids {
		values := []any{[]any{}}
		columns := []Column{{Name: "values", OID: oid}}
		require.NoError(t, NormalizeRow(columns, values), "oid %d", oid)

		writer, err := NewWriter(io.Discard, columns)
		require.NoError(t, err)
		require.NoError(t, writer.WriteRow(values), "oid %d", oid)
		require.NoError(t, writer.Close())
	}
}

func TestNormalizeEnumArrayForCopy(t *testing.T) {
	values := []any{[]any{
		sqltypes.EnumValue{Number: 1, Name: "TRANSFER"},
		sqltypes.EnumValue{Number: 7},
	}}
	columns := []Column{{Name: "kinds", OID: pgtype.TextArrayOID}}

	require.NoError(t, NormalizeRow(columns, values))
	require.Equal(t, []string{"TRANSFER", "7"}, values[0])

	writer, err := NewWriter(io.Discard, columns)
	require.NoError(t, err)
	require.NoError(t, writer.WriteRow(values))
	require.NoError(t, writer.Close())
}

func numericFromString(value string) (pgtype.Numeric, error) {
	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		return pgtype.Numeric{}, err
	}

	return numeric, nil
}
