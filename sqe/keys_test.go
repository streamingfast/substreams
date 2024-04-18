package sqe

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyKeys(t *testing.T) {
	kv := map[string]struct{}{
		"bob":      {},
		"alice":    {},
		"etienne":  {},
		"charlie":  {},
		"delegate": {},
		"mint":     {},
	}

	blockKeys := KeysQuerier{blockKeys: kv}

	// Matrix-based test cases
	testCases := []struct {
		name   string
		expr   string
		result bool
	}{
		{
			name:   "Or",
			expr:   "bob || alice",
			result: true,
		},
		{
			name:   "And",
			expr:   "bob transfer",
			result: false,
		},
		{
			name:   "And(Or key)",
			expr:   "(alice || bob) transfer",
			result: false,
		},
		{
			name:   "And(Or Or)",
			expr:   "(alice || bob) (delegate || mint)",
			result: true,
		},

		{
			name:   "2 And",
			expr:   "alice john mint",
			result: false,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parser, err := NewParser(strings.NewReader(tc.expr))
			require.NoError(t, err)

			expr, err := parser.Parse(context.Background())
			require.NoError(t, err)

			assert.Equal(t, tc.result, blockKeys.apply(expr))
		})
	}
}
