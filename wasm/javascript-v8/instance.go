package v8

import (
	"context"

	"rogchap.com/v8go"
)

type V8Instance struct {
	iso *v8go.Isolate
	ctx *v8go.Context
}

func NewV8Instance(iso *v8go.Isolate) (*V8Instance, error) {
	ctx := v8go.NewContext(iso)
	return &V8Instance{iso, ctx}, nil
}

func (inst *V8Instance) Cleanup(ctx context.Context) error {
	return nil
}

func (inst *V8Instance) Close(ctx context.Context) error {
	inst.ctx.Close()
	inst.ctx = nil
	inst.iso = nil
	return nil
}
