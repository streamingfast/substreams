package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrimaryKey(t *testing.T) {
	tests := []struct {
		name        string
		in          []*ColumnInfo
		expectOut   map[string]string
		expectError bool
	}{
		{
			name:        "no primkey error",
			expectError: true,
		},
		{
			name: "more than one primkey error",
			in: []*ColumnInfo{
				{
					name: "one",
				},
				{
					name: "two",
				},
			},
			expectError: true,
		},
		{
			name: "single than primkey ok",
			in: []*ColumnInfo{
				{
					name: "id",
				},
			},
			expectOut: map[string]string{
				"id": "testval",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			l := &Loader{
				tables: map[string]*TableInfo{
					"test": {
						primaryColumns: test.in,
					},
				},
			}
			out, err := l.GetPrimaryKey("test", "testval")
			if test.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectOut, out)
			}

		})
	}

}

func TestLoader_PendingOperationsMerge(t *testing.T) {
	type change struct {
		value              string
		reversibleBlockNum *uint64
	}

	tests := []struct {
		name               string
		handleReorgs       bool
		changes            []change
		expectedOperations int
	}{
		{"same reversible block merged", true, []change{{"1", new(uint64(10))}, {"2", new(uint64(10))}}, 1},
		{"different reversible blocks kept separate", true, []change{{"1", new(uint64(10))}, {"2", new(uint64(11))}}, 2},
		{"final then reversible block kept separate", true, []change{{"1", nil}, {"2", new(uint64(9))}}, 2},
		{"final blocks merged", true, []change{{"1", nil}, {"2", nil}}, 1},
		{"without reorg handling, different reversible blocks merged", false, []change{{"1", new(uint64(10))}, {"2", new(uint64(11))}}, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			l := NewTestLoader(t, "psql://x:5432/x?schemaName=testschema", nil, TestSinglePrimaryKeyTables("testschema"), zlog, tracer)
			l.handleReorgs = test.handleReorgs

			for _, change := range test.changes {
				require.NoError(t, l.Upsert("xfer", map[string]string{"id": "1234"}, map[string]FieldData{"from": {Value: change.value}}, change.reversibleBlockNum))
			}

			entries, found := l.entries.Get("xfer")
			require.True(t, found)
			assert.Equal(t, test.expectedOperations, entries.Len())
		})
	}
}
