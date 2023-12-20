package wasi

import (
	"context"
)

type instance struct {
}

type allocation struct {
	ptr    uint32
	length uint32
}

func (i *instance) Cleanup(ctx context.Context) error {
	return nil
}

func (i *instance) Close(ctx context.Context) error {
	return nil
}

func instanceFromContext(ctx context.Context) *instance {
	return ctx.Value("instance").(*instance)
}
func withInstanceContext(ctx context.Context, inst *instance) context.Context {
	return context.WithValue(ctx, "instance", inst)
}
