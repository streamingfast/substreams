package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

func NewInstance(mod api.Module, memFuncs runtimeSauce) *Instance {
	return &Instance{
		Module:       mod,
		runtimeSauce: memFuncs,
	}
}

type Instance struct {
	api.Module
	allocations  []allocation
	runtimeSauce runtimeSauce
}

type allocation struct {
	ptr    uint32
	length uint32
}

func (i *Instance) Cleanup(ctx context.Context) error {
	deallocate(ctx, i)
	return nil
}

func (i *Instance) Close(ctx context.Context) error {
	return i.Module.Close(ctx)
}

func instanceFromContext(ctx context.Context) *Instance {
	return ctx.Value("instance").(*Instance)
}
func WithInstanceContext(ctx context.Context, inst *Instance) context.Context {
	return context.WithValue(ctx, "instance", inst)
}
