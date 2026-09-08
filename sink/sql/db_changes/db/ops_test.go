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
