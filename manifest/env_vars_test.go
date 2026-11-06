package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("MANIFEST_TEST_SET", "value")
	t.Setenv("MANIFEST_TEST_EMPTY", "")

	tests := []struct {
		name       string
		in         string
		expected   string
		errSubstrs []string
	}{
		{"no reference", "plain-value", "plain-value", nil},
		{"set var short", "$MANIFEST_TEST_SET", "value", nil},
		{"set var braces", "id-${MANIFEST_TEST_SET}-suffix", "id-value-suffix", nil},
		{"empty var is valid", "[${MANIFEST_TEST_EMPTY}]", "[]", nil},
		{"missing var", "$MANIFEST_TEST_MISSING", "", []string{"MANIFEST_TEST_MISSING"}},
		{"multiple missing", "$MANIFEST_TEST_A-${MANIFEST_TEST_B}", "", []string{"MANIFEST_TEST_A", "MANIFEST_TEST_B"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := expandEnvVars(tt.in)
			if tt.errSubstrs != nil {
				require.Error(t, err)
				for _, substr := range tt.errSubstrs {
					assert.Contains(t, err.Error(), substr)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}
