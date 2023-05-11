package wasm

import (
	"context"
)

func withContext(ctx context.Context, call *Call) context.Context {
	return context.WithValue(ctx, "call", call)
}

func fromContext(ctx context.Context) *Call {
	return ctx.Value("call").(*Call)
}
