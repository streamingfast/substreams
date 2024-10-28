package reqctx

import (
	"context"

	"google.golang.org/grpc/metadata"
)

var backFillerKey = "livebackfiller"

func WithBackfillerRequest(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, backFillerKey, "true")
}

func HasBackfillerRequest(ctx context.Context) bool {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return false
	}

	_, ok = md[backFillerKey]
	return ok
}

func IsBackfillerRequest(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}

	_, ok = md[backFillerKey]
	return ok
}

type outputModuleKeyType int

func WithOutputModuleHash(ctx context.Context, hash string) context.Context {
	return context.WithValue(ctx, outputModuleHashKey, hash)
}

func OutputModuleHash(ctx context.Context) string {
	hash, ok := ctx.Value(outputModuleHashKey).(string)
	if !ok {
		return ""
	}
	return hash
}
