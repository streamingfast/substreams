package clickhouse

import (
	"testing"

	sql2 "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/stretchr/testify/require"
)

func TestEncodeValuesSupportsEnumValue(t *testing.T) {
	encoded, err := encodeValues([]any{sql2.EnumValue{Number: 2, Name: "ACTIVE"}})
	require.NoError(t, err)

	decoded, err := decodeValues(encoded)
	require.NoError(t, err)
	require.Equal(t, []any{int32(2)}, decoded)
}
