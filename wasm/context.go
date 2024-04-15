package wasm

import (
	"context"
)

type callCtxType string

const callCtx = callCtxType("call")

func WithContext(ctx context.Context, call *Call) context.Context {
	return context.WithValue(ctx, callCtx, call)
}

func FromContext(ctx context.Context) *Call {
	return ctx.Value(callCtx).(*Call)
}
