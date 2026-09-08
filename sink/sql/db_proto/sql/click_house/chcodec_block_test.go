package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEncodeValuesRoundTripsABlockRow covers the row shape InsertBlock produces, now that
// it goes through the spool like every other one. A tag missing here would fail the run at
// spool time rather than silently, but only once a block is written — which is every block.
func TestEncodeValuesRoundTripsABlockRow(t *testing.T) {
	timestamp := time.Unix(1700000000, 0).UTC()
	row := []any{uint64(42), "0xabc", timestamp, int64(7), false}

	encoded, err := encodeValues(row)
	require.NoError(t, err)

	decoded, err := decodeValues(encoded)
	require.NoError(t, err)
	require.Equal(t, row, decoded)
}
