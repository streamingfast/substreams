package db

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The row interval flushes once more rows than it are pending, and 0 turns it off, leaving the
// block interval alone to decide (see --batch-row-flush-interval).
func TestLoader_FlushNeeded(t *testing.T) {
	for _, test := range []struct {
		rowInterval int
		rows        int
		expected    bool
	}{
		{rowInterval: 0, rows: 0, expected: false},
		{rowInterval: 0, rows: 5, expected: false},
		{rowInterval: 3, rows: 3, expected: false},
		{rowInterval: 3, rows: 4, expected: true},
	} {
		t.Run(fmt.Sprintf("interval %d, %d rows", test.rowInterval, test.rows), func(t *testing.T) {
			l := NewTestLoader(t, "psql://x:5432/x?schemaName=testschema", nil, TestSinglePrimaryKeyTables("testschema"), zlog, tracer)
			l.SetFlushIntervals(1000, test.rowInterval, 1)
			for i := range test.rows {
				require.NoError(t, l.Insert("xfer", map[string]string{"id": fmt.Sprintf("%d", i)}, map[string]FieldData{}, nil))
			}
			assert.Equal(t, test.expected, l.FlushNeeded())
		})
	}
}
