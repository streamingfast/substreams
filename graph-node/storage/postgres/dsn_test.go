package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDSN(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		in          string
		expected    string
		expectedErr error
	}{
		{
			"standard",
			map[string]string{"PG_PASSWORD": "a"},
			"postgresql://graph:${PG_PASSWORD}@127.0.0.1:5432/graph?enable_incremental_sort=off&sslmode=disable",
			"host=127.0.0.1 port=5432 user=graph dbname=graph enable_incremental_sort=off sslmode=disable password=a",
			nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := parseDSN(test.in, func(s string) string { return test.env[s] })
			if test.expectedErr == nil {
				require.NoError(t, err, "Unexpected error on %q", test.in)

				dsn := actual.DSN()
				assert.Equal(t, test.expected, dsn, "Mistmatch DSN from %q", test.in)
			} else {
				assert.Equal(t, test.expectedErr, err, "Invalid error on %q", test.in)
			}
		})
	}
}
