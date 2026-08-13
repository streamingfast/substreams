package pgcopy

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
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

func numericFromString(value string) (pgtype.Numeric, error) {
	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		return pgtype.Numeric{}, err
	}

	return numeric, nil
}
