package wasm

import (
	"fmt"
	"sort"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v4"
)

type allocation struct {
	ptr    int32
	length int
}

type Heap struct {
	allocations []*allocation
	memory      *wasmtime.Memory
	allocator   *wasmtime.Func
	dealloc     *wasmtime.Func
	store       *wasmtime.Store
}

func NewHeap(memory *wasmtime.Memory, allocator, dealloc *wasmtime.Func, store *wasmtime.Store) *Heap {
	return &Heap{
		memory:    memory,
		allocator: allocator,
		dealloc:   dealloc,
		store:     store,
	}
}

func (h *Heap) Write(bytes []byte, from string) (int32, error) {
	return h.WriteAndTrack(bytes, true, from)
}

func (h *Heap) WriteAndTrack(bytes []byte, track bool, from string) (int32, error) {
	size := len(bytes)
	results, err := h.allocator.Call(h.store, int32(size))
	if err != nil {
		return 0, fmt.Errorf("allocating memory for size %d:%w", size, err)
	}

	ptr := results.(int32)
	if track {
		h.allocations = append(h.allocations, &allocation{ptr: ptr, length: len(bytes)})
	}
	return h.WriteAtPtr(bytes, ptr, from)
}

func (h *Heap) WriteAtPtr(bytes []byte, ptr int32, from string) (int32, error) {
	data := h.memory.UnsafeData(h.store)
	copy(data[ptr:], bytes)
	return ptr, nil
}

func (h *Heap) Clear() error {
	sort.Slice(h.allocations, func(i, j int) bool {
		return h.allocations[i].ptr < h.allocations[j].ptr
	})
	for _, a := range h.allocations {
		if _, err := h.dealloc.Call(h.store, a.ptr, int32(a.length)); err != nil {
			return fmt.Errorf("deallocating memory at ptr %d: %w", a.ptr, err)
		}
	}
	h.allocations = nil
	return nil
}

func (h *Heap) ReadString(ptr int32, length int32) string {
	data := h.ReadBytes(ptr, length)
	return string(data)
}

func (h *Heap) ReadBytes(ptr int32, length int32) []byte {
	data := h.memory.UnsafeData(h.store)
	return data[ptr : ptr+length]
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
