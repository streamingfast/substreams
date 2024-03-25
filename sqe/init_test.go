package sqe

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func expressionToString(expression Expression) string {
	builder := &strings.Builder{}
	visitor := &TestVisitor{
		writer: builder,
	}

	expression.Visit(context.Background(), visitor)

	return builder.String()
}

type TestVisitor struct {
	writer io.Writer
}

func (v *TestVisitor) Visit_And(ctx context.Context, e *AndExpression) error {
	return v.visit_binary(ctx, "<", "&&", ">", e.Children)
}

func (v *TestVisitor) Visit_Or(ctx context.Context, e *OrExpression) error {
	return v.visit_binary(ctx, "[", "||", "]", e.Children)
}

func (v *TestVisitor) visit_binary(ctx context.Context, opStart, op, opEnd string, children []Expression) error {
	v.print(opStart)

	for i, child := range children {
		if i != 0 {
			v.print(" %s ", op)
		}

		child.Visit(ctx, v)
	}
	v.print(opEnd)

	return nil
}

func (v *TestVisitor) Visit_Parenthesis(ctx context.Context, e *ParenthesisExpression) error {
	v.print("(")
	e.Child.Visit(ctx, v)
	v.print(")")

	return nil
}

func (v *TestVisitor) Visit_Not(ctx context.Context, e *NotExpression) error {
	v.print("!")
	e.Child.Visit(ctx, v)

	return nil
}

func (v *TestVisitor) Visit_KeyTerm(ctx context.Context, e *KeyTerm) error {
	v.printStringLiteral(e.Value)
	return nil
}

func (v *TestVisitor) printStringLiteral(literal *StringLiteral) error {
	if literal.QuotingChar != "" {
		return v.print("%s%s%s", literal.QuotingChar, literal.Value, literal.QuotingChar)
	}

	return v.print(literal.Value)
}

func (v *TestVisitor) print(message string, args ...interface{}) error {
	fmt.Fprintf(v.writer, message, args...)
	return nil
}
