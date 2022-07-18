package wasm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

type Heap struct {
	//memory    api.Memory
	allocator api.Function
	dealloc   api.Function
}

func NewHeap(allocator, dealloc api.Function) *Heap {
	return &Heap{
		allocator: allocator,
		dealloc:   dealloc,
	}
}

func (h *Heap) Write(ctx context.Context, memory api.Memory, bytes []byte) (uint32, error) {
	size := len(bytes)

	results, err := h.allocator.Call(ctx, uint64(size))
	if err != nil {
		return 0, fmt.Errorf("allocating memory for size %d:%w", size, err)
	}
	ptr := results[0]

	// This pointer is managed by TinyGo, but TinyGo is unaware of external usage.
	// So, we have to free it when finished defer h.free.Call(ctx, ptr)
	defer h.dealloc.Call(ctx, ptr)

	return h.WriteAtPtr(ctx, memory, bytes, uint32(ptr))
}
func (h *Heap) WriteAtPtr(ctx context.Context, memory api.Memory, bytes []byte, ptr uint32) (uint32, error) {
	if !memory.Write(ctx, ptr, bytes) {
		return 0, fmt.Errorf("failed writing to memory at ptr %d", ptr)
	}

	return ptr, nil
}

func (h *Heap) ReadString(ctx context.Context, memory api.Memory, ptr uint32, length uint32) (string, error) {
	data, ok := memory.Read(ctx, ptr, length)
	if !ok {
		return "", fmt.Errorf("failed reading string from memory at ptr %d", ptr)
	}
	return string(data), nil
}

func (h *Heap) ReadBytes(ctx context.Context, memory api.Memory, ptr uint32, length uint32) ([]byte, error) {
	data, ok := memory.Read(ctx, ptr, length)
	if !ok {
		return nil, fmt.Errorf("failed reading bytes from memory at ptr %d", ptr)
	}

	return data, nil
}

//func (h *Heap) PrintMem() {
//	data := h.memory.Data()
//	for i, datum := range data {
//		if i > 1024 {
//			if datum == 0 {
//				continue
//			}
//		}
//		fmt.Print(datum, ", ")
//	}
//	fmt.Print("\n")
//}
