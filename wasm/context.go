package wasm

import (
	"context"
)

func WithContext(ctx context.Context, call *Call) context.Context {
	return context.WithValue(ctx, "call", call)
}

func FromContext(ctx context.Context) *Call {
	return ctx.Value("call").(*Call)
}
