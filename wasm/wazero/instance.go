package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

type instance struct {
	api.Module
	allocations []allocation
}

type allocation struct {
	ptr    uint32
	length uint32
}

func (i *instance) Cleanup(ctx context.Context) error {
	deallocate(ctx, i)
	return nil
}

func (i *instance) Close(ctx context.Context) error {
	return i.Module.Close(ctx)
}

func instanceFromContext(ctx context.Context) *instance {
	return ctx.Value("instance").(*instance)
}
func withInstanceContext(ctx context.Context, inst *instance) context.Context {
	return context.WithValue(ctx, "instance", inst)
}
