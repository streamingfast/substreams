package wasm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

type allocation struct {
	ptr    uint32
	length int
}

type Heap struct {
	allocations []*allocation
	allocator   api.Function
	dealloc     api.Function
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

	return h.WriteAtPtr(ctx, memory, bytes, uint32(ptr))
}
func (h *Heap) WriteAtPtr(ctx context.Context, memory api.Memory, bytes []byte, ptr uint32) (uint32, error) {
	if !memory.Write(ctx, ptr, bytes) {
		return 0, fmt.Errorf("failed writing to memory at ptr %d", ptr)
	}
	h.allocations = append(h.allocations, &allocation{ptr: ptr, length: len(bytes)})
	return ptr, nil
}

func (h *Heap) Clear(ctx context.Context) error {
	for _, a := range h.allocations {
		if _, err := h.dealloc.Call(ctx, uint64(a.ptr), uint64(a.length)); err != nil {
			return fmt.Errorf("deallocating memory at ptr %d: %w", a.ptr, err)
		}
	}
	h.allocations = nil
	return nil
}

func (h *Heap) ReadString(ctx context.Context, memory api.Memory, ptr uint32, length uint32) (string, error) {
	data, err := h.ReadBytes(ctx, memory, ptr, length)
	if err != nil {
		return "", fmt.Errorf("reading bytes from memory at ptr %d: %w", ptr, err)
	}
	return string(data), nil
}

func (h *Heap) ReadBytes(ctx context.Context, memory api.Memory, ptr uint32, length uint32) ([]byte, error) {
	data, ok := memory.Read(ctx, ptr, length)
	if !ok {
		return nil, fmt.Errorf("failed reading bytes from memory at ptr %d", ptr)
	}

	out := make([]byte, length)
	copy(out, data)
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
