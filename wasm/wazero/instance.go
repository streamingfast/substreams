package wazero

import (
	"context"
	"fmt"
	"sort"
	"time"

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

func (i *instance) deallocate(ctx context.Context) {
	t0 := time.Now()
	sort.Slice(i.allocations, func(j, k int) bool {
		return i.allocations[j].ptr < i.allocations[k].ptr
	})
	dealloc := i.ExportedFunction("dealloc")
	for _, alloc := range i.allocations {
		fmt.Println("  dealloc", alloc.ptr, alloc.length)
		if err := dealloc.CallWithStack(ctx, []uint64{uint64(alloc.ptr), uint64(alloc.length)}); err != nil {
			panic(fmt.Errorf("could not deallocate %d bytes from memory at %d: %w", alloc.length, alloc.ptr, err))
		}
	}
	fmt.Println("deallocate took", time.Since(t0), len(i.allocations), i.Memory().Size())
	i.allocations = nil
}

func (i *instance) Cleanup(ctx context.Context) error {
	i.deallocate(ctx)
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
