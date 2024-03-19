package sqe

import (
	"context"
	"fmt"
)

func optimizeExpression(ctx context.Context, expr Expression) Expression {
	visitor := NewDepthFirstVisitor(nil, func(_ context.Context, expr Expression) error {
		v, ok := expr.(*OrExpression)
		if !ok {
			return nil
		}

		newChildren := make([]Expression, 0, len(v.Children))
		for _, child := range v.Children {
			if w, ok := child.(*OrExpression); ok {
				newChildren = append(newChildren, w.Children...)
			} else {
				newChildren = append(newChildren, child)
			}
		}

		v.Children = newChildren
		return nil
	})

	if err := expr.Visit(ctx, visitor); err != nil {
		panic(fmt.Errorf("optimizer visitor is never expected to return error, something changed: %w", err))
	}

	return expr
}
