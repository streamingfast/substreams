package wazero

import (
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

func writeToHeap(ctx context.Context, inst *instance, data []byte) (uint32, error) {
	size := len(data)
	stack := []uint64{uint64(len(data))}
	if err := inst.ExportedFunction("alloc").CallWithStack(ctx, stack); err != nil {
		return 0, fmt.Errorf("alloc from: %w", err)
	}
	ptr := uint32(stack[0])
	if ok := inst.Memory().Write(ptr, data); !ok {
		return 0, fmt.Errorf("could not write to memory")
	}
	fmt.Println("Memory size:", inst.Memory().Size(), ptr, size)
	if size != 0 {
		inst.allocations = append(inst.allocations, allocation{ptr: ptr, length: uint32(size)})
	}
	return ptr, nil
}

func writeOutputToHeap(ctx context.Context, inst *instance, outputPtr uint32, value []byte) error {
	valuePtr, err := writeToHeap(ctx, inst, value)
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
