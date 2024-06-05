package sqe

import (
	"context"
	"errors"
)

type OnExpression func(ctx context.Context, expr Expression) error

var ErrStopVisit = errors.New("stop")

type DepthFirstVisitor struct {
	beforeVisit OnExpression
	afterVisit  OnExpression
	stopped     bool
}

func NewDepthFirstVisitor(beforeVisit, afterVisit OnExpression) *DepthFirstVisitor {
	return &DepthFirstVisitor{beforeVisit: beforeVisit, afterVisit: afterVisit}
}

func (v *DepthFirstVisitor) Visit_And(ctx context.Context, e *AndExpression) error {
	return v.visit_binary(ctx, e, e.Children)
}

func (v *DepthFirstVisitor) Visit_Or(ctx context.Context, e *OrExpression) error {
	return v.visit_binary(ctx, e, e.Children)
}

func (v *DepthFirstVisitor) visit_binary(ctx context.Context, parent Expression, children []Expression) error {
	if stop, err := v.executeCallback(ctx, parent, v.beforeVisit); stop {
		return err
	}

	for _, child := range children {
		err := child.Visit(ctx, v)
		if v.stopped || err != nil {
			return err
		}
	}

	if stop, err := v.executeCallback(ctx, parent, v.afterVisit); stop {
		return err
	}

	return nil
}

func (v *DepthFirstVisitor) Visit_Parenthesis(ctx context.Context, e *ParenthesisExpression) error {
	if stop, err := v.executeCallback(ctx, e, v.beforeVisit); stop {
		return err
	}

	if err := e.Child.Visit(ctx, v); err != nil {
		return err
	}

	if stop, err := v.executeCallback(ctx, e, v.afterVisit); stop {
		return err
	}

	return nil
}

func (v *DepthFirstVisitor) Visit_Not(ctx context.Context, e *NotExpression) error {
	if stop, err := v.executeCallback(ctx, e, v.beforeVisit); stop {
		return err
	}

	if err := e.Child.Visit(ctx, v); err != nil {
		return err
	}

	if stop, err := v.executeCallback(ctx, e, v.afterVisit); stop {
		return err
	}

	return nil
}

func (v *DepthFirstVisitor) Visit_KeyTerm(ctx context.Context, e *KeyTerm) error {
	if stop, err := v.executeCallback(ctx, e, v.beforeVisit); stop {
		return err
	}

	if stop, err := v.executeCallback(ctx, e, v.afterVisit); stop {
		return err
	}

	return nil
}

func (v *DepthFirstVisitor) executeCallback(ctx context.Context, e Expression, callback OnExpression) (stop bool, err error) {
	if callback == nil {
		return false, nil
	}

	if v.stopped {
		return true, nil
	}

	if err := callback(ctx, e); err != nil {
		if err == ErrStopVisit {
			v.stopped = true
			return true, nil
		} else {
			v.stopped = true
			return true, err
		}
	}

	return false, nil
}
