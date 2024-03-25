package sqe

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptimizer(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expression
		expected string
	}{
		{
			"top_or_no_or_children",
			orExpr(keyTermExpr("a1"), keyTermExpr("a2")),
			`[a1 || a2]`,
		},
		{
			"top_or_single_or_children",
			orExpr(orExpr(keyTermExpr("a1"), keyTermExpr("a2")), keyTermExpr("b2")),
			`[a1 || a2 || b2]`,
		},
		{
			"top_or_multiple_or_children",
			orExpr(
				orExpr(keyTermExpr("a1"), keyTermExpr("a2")),
				orExpr(keyTermExpr("c1"), keyTermExpr("c2")),
			),
			`[a1 || a2 || c1 || c2]`,
		},
		{
			"top_or_mixed_multiple_or_children",
			orExpr(
				keyTermExpr("before2"),
				orExpr(keyTermExpr("a1"), keyTermExpr("a2")),
				andExpr(keyTermExpr("middle1"), keyTermExpr("middle2")),
				orExpr(keyTermExpr("c1"), keyTermExpr("c2")),
				notExpr(keyTermExpr("after3")),
			),
			`[before2 || a1 || a2 || <middle1 && middle2> || c1 || c2 || !after3]`,
		},

		{
			"or_in_not_multiple_or_children",
			notExpr(
				orExpr(
					orExpr(keyTermExpr("a1"), keyTermExpr("a2")),
					orExpr(keyTermExpr("c1"), keyTermExpr("c2")),
				),
			),
			`![a1 || a2 || c1 || c2]`,
		},
		{
			"or_in_parens_multiple_or_children",
			parensExpr(
				orExpr(
					orExpr(keyTermExpr("a1"), keyTermExpr("a2")),
					orExpr(keyTermExpr("c1"), keyTermExpr("c2")),
				),
			),
			`([a1 || a2 || c1 || c2])`,
		},

		{
			"multi_level_nested_only_or",
			orExpr(
				orExpr(
					orExpr(
						keyTermExpr("l3a1"),
						orExpr(keyTermExpr("l4a1"), keyTermExpr("l4a2")),
					),
					orExpr(
						orExpr(keyTermExpr("l4b1"), keyTermExpr("l4b2")),
						keyTermExpr("l3b1"),
					),
					orExpr(
						orExpr(keyTermExpr("l4c1"), keyTermExpr("l4c2")),
						orExpr(keyTermExpr("l4d1"), keyTermExpr("l4d2")),
					),
				),
			),
			`[l3a1 || l4a1 || l4a2 || l4b1 || l4b2 || l3b1 || l4c1 || l4c2 || l4d1 || l4d2]`,
		},

		{
			"multi_level_nested_mixed_or",
			orExpr(
				orExpr(
					andExpr(
						keyTermExpr("l3a1"),
						notExpr(orExpr(keyTermExpr("l4a1"), keyTermExpr("l4a2"))),
					),
					orExpr(
						orExpr(keyTermExpr("l4b1"), keyTermExpr("l4b2")),
						keyTermExpr("l3b1"),
					),
					orExpr(
						orExpr(keyTermExpr("l4c1"), keyTermExpr("l4c2")),
						parensExpr(orExpr(keyTermExpr("l4d1"), keyTermExpr("l4d2"))),
					),
				),
				andExpr(
					keyTermExpr("l2e1"),
					orExpr(keyTermExpr("l3f1"), keyTermExpr("l3f2")),
				),
			),
			`[<l3a1 && ![l4a1 || l4a2]> || l4b1 || l4b2 || l3b1 || l4c1 || l4c2 || ([l4d1 || l4d2]) || <l2e1 && [l3f1 || l3f2]>]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			optimized := optimizeExpression(context.Background(), test.expr)
			assert.Equal(t, test.expected, expressionToString(optimized), "Invalid optimization for %q", test.name)
		})
	}
}
