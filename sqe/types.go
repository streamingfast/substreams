package sqe

import (
	"context"
	"fmt"
)

type Visitor interface {
	Visit_And(ctx context.Context, expr *AndExpression) error
	Visit_Or(ctx context.Context, expr *OrExpression) error
	Visit_Parenthesis(ctx context.Context, expr *ParenthesisExpression) error
	Visit_Not(ctx context.Context, expr *NotExpression) error
	Visit_KeyTerm(ctx context.Context, expr *KeyTerm) error
}

type Expression interface {
	Visit(ctx context.Context, visitor Visitor) error
}

type HasChildrenExpression interface {
	GetChildren() []Expression
}

type AndExpression struct {
	Children []Expression
}

func andExpr(children ...Expression) *AndExpression {
	return &AndExpression{Children: children}
}

func (e *AndExpression) Visit(ctx context.Context, visitor Visitor) error {
	return visitor.Visit_And(ctx, e)
}

func (e *AndExpression) GetChildren() []Expression {
	return e.Children
}

type OrExpression struct {
	Children []Expression
}

func orExpr(children ...Expression) *OrExpression {
	return &OrExpression{Children: children}
}

func (e *OrExpression) Visit(ctx context.Context, visitor Visitor) error {
	return visitor.Visit_Or(ctx, e)
}

func (e *OrExpression) GetChildren() []Expression {
	return e.Children
}

type ParenthesisExpression struct {
	Child Expression
}

func parensExpr(expr Expression) *ParenthesisExpression {
	return &ParenthesisExpression{Child: expr}
}

func (e *ParenthesisExpression) Visit(ctx context.Context, visitor Visitor) error {
	return visitor.Visit_Parenthesis(ctx, e)
}

type NotExpression struct {
	Child Expression
}

func notExpr(expr Expression) *NotExpression {
	return &NotExpression{Child: expr}
}

func (e *NotExpression) Visit(ctx context.Context, visitor Visitor) error {
	return visitor.Visit_Not(ctx, e)
}

type KeyTerm struct {
	Value *StringLiteral
}

func keyTermExpr(value string) *KeyTerm {
	return &KeyTerm{Value: &StringLiteral{Value: value}}
}

func (e *KeyTerm) Visit(ctx context.Context, visitor Visitor) error {
	return visitor.Visit_KeyTerm(ctx, e)
}

type StringLiteral struct {
	Value       string
	QuotingChar string
}

func (e *StringLiteral) Literal() string {
	return e.Value
}

func (e *StringLiteral) SetValue(value string) {
	e.Value = value
}

func (e *StringLiteral) String() string {
	if e.QuotingChar != "" {
		return fmt.Sprintf("%s%s%s", e.QuotingChar, e.Value, e.QuotingChar)
	}

	return e.Value
}
