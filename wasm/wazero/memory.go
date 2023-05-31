package wazero

import (
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

func writeToHeap(ctx context.Context, inst *instance, track bool, data []byte) (uint32, error) {
	size := len(data)
	stack := []uint64{uint64(size)}
	if err := inst.ExportedFunction("alloc").CallWithStack(ctx, stack); err != nil {
		return 0, fmt.Errorf("alloc from: %w", err)
	}
	ptr := uint32(stack[0])
	if ok := inst.Memory().Write(ptr, data); !ok {
		return 0, fmt.Errorf("could not write to memory")
	}
	//fmt.Println("  writeToHeap/alloc:", inst.Memory().Size(), ptr, size)
	if track && size != 0 {
		inst.allocations = append(inst.allocations, allocation{ptr: ptr, length: uint32(size)})
	}
	return ptr, nil
}

func writeOutputToHeap(ctx context.Context, inst *instance, outputPtr uint32, value []byte) error {
	valuePtr, err := writeToHeap(ctx, inst, false, value)
	if err != nil {
		return fmt.Errorf("writing value: %w", err)
	}
	mem := inst.Memory()
	if ok := mem.WriteUint32Le(outputPtr, valuePtr); !ok {
		return errors.New("writing WriteUint32Le:1 to memory")
	}
	if ok := mem.WriteUint32Le(outputPtr+4, uint32(len(value))); !ok {
		return errors.New("writing WriteUint32Le:2 to memory")
	}
	return nil
}

func deallocate(ctx context.Context, i *instance) {
	//t0 := time.Now()
	dealloc := i.ExportedFunction("dealloc")
	for _, alloc := range i.allocations {
		//fmt.Println("  dealloc", alloc.ptr, alloc.length)
		if err := dealloc.CallWithStack(ctx, []uint64{uint64(alloc.ptr), uint64(alloc.length)}); err != nil {
			panic(fmt.Errorf("could not deallocate %d bytes from memory at %d: %w", alloc.length, alloc.ptr, err))
		}
	}
	//fmt.Println("deallocate took", time.Since(t0), len(i.allocations), i.Memory().Size())
	i.allocations = nil
}

func readBytesFromStack(mod api.Module, stack []uint64) []byte {
	ptr, length := uint32(stack[0]), uint32(stack[1])
	return readBytes(mod, ptr, length)
}
func readStringFromStack(mod api.Module, stack []uint64) string {
	ptr, length := uint32(stack[0]), uint32(stack[1])
	return readString(mod, ptr, length)
}

func readString(mod api.Module, ptr, len uint32) string {
	bytes, ok := mod.Memory().Read(ptr, len)
	if !ok {
		panic(fmt.Sprintf("could not read string, ptr=%d, len=%d", ptr, len))
	}
	return string(bytes)
}

func readBytes(mod api.Module, ptr, length uint32) []byte {
	bytes, ok := mod.Memory().Read(ptr, length)
	if !ok {
		panic(fmt.Sprintf("could not read string, ptr=%d, len=%d", ptr, length))
	}
	return bytes
}
