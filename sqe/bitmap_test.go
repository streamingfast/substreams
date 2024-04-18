package sqe

import (
	"context"
	"strings"
	"testing"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyRoaringBitmap(t *testing.T) {
	kv := map[string]*roaring64.Bitmap{
		"bob":      roaring64.BitmapOf(1, 2, 3),
		"alice":    roaring64.BitmapOf(1, 4, 5),
		"john":     roaring64.BitmapOf(1, 3, 5),
		"transfer": roaring64.BitmapOf(1, 3, 5),
		"mint":     roaring64.BitmapOf(5),
		"delegate": roaring64.BitmapOf(4),
	}

	// Matrix-based test cases
	testCases := []struct {
		expr      string
		operation func() *roaring64.Bitmap
		result    []uint64
	}{
		{
			expr:   "bob || alice",
			result: []uint64{1, 2, 3, 4, 5},
		},
		{
			expr:   "bob transfer",
			result: []uint64{1, 3},
		},
		{
			expr:   "(alice || bob) transfer",
			result: []uint64{1, 3, 5},
		},
		{
			expr:   "(alice || bob) (delegate || mint)",
			result: []uint64{4, 5},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		parser, err := NewParser(strings.NewReader(tc.expr))
		require.NoError(t, err)

		expr, err := parser.Parse(context.Background())
		require.NoError(t, err)

		assert.ElementsMatch(t, tc.result, RoaringBitmapsApply(expr, kv).ToArray())
	}
}
