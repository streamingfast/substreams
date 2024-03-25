package sqe

import (
	"context"
	"fmt"
)

// FindAllFieldNames returns all used field names in the AST. There
// is **NO** ordering on the elements, i.e. they might not come in the
// same order specified in the AST.
func ExtractAllKeys(expression Expression) (out []string) {
	uniqueFieldNames := map[string]bool{}
	onExpression := func(_ context.Context, expr Expression) error {
		if v, ok := expr.(*KeyTerm); ok {
			uniqueFieldNames[v.Value.Value] = true
		}

		return nil
	}

	visitor := NewDepthFirstVisitor(nil, onExpression)
	expression.Visit(context.Background(), visitor)

	i := 0
	out = make([]string, len(uniqueFieldNames))
	for fieldName := range uniqueFieldNames {
		out[i] = fieldName
		i++
	}

	return
}

func TransformExpression(expr Expression, transformer FieldTransformer) error {
	if transformer == nil {
		return nil
	}

	onExpression := func(_ context.Context, expr Expression) error {
		v, ok := expr.(*KeyTerm)
		if !ok {
			return nil
		}

		if err := transformer.TransformStringLiteral("", v.Value); err != nil {
			return fmt.Errorf("key %q transformation failed: %s", v.Value.Value, err)
		}

		return nil
	}

	visitor := NewDepthFirstVisitor(nil, onExpression)
	return expr.Visit(context.Background(), visitor)
}
