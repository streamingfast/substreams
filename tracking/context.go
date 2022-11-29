package tracking

import "context"

type contextKeyType int

var bytesMeterKey = contextKeyType(-1)

func WithBytesMeter(ctx context.Context, meter BytesMeter) context.Context {
	return context.WithValue(ctx, bytesMeterKey, meter)
}

func GetBytesMeter(ctx context.Context) BytesMeter {
	meter := ctx.Value(bytesMeterKey)
	if m, ok := meter.(BytesMeter); ok {
		return m
	}
	return NoopBytesMeter
}
