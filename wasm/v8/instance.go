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

// Unused right now and simply comes from the wasm interface but might
// be useful when cleaning between requests
func (inst *V8Instance) Cleanup(ctx context.Context) error {
	return nil
}

func (inst *V8Instance) Close(ctx context.Context) error {
	inst.ctx.Close()
	return nil
}
