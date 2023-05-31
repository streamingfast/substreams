package wasmtime

import (
	"encoding/binary"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go/v4"
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

func writeOutputToHeap(i *instance, outputPtr int32, value []byte) error {
	valuePtr, err := i.Heap.WriteAndTrack(value, false, "WriteOutputToHeap1")
	if err != nil {
		return fmt.Errorf("writing value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], uint32(valuePtr))
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = i.Heap.WriteAtPtr(returnValue, outputPtr, "WriteOutputToHeap2")
	if err != nil {
		return fmt.Errorf("writing pointer %d to heap: %w", valuePtr, err)
	}
	return nil
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

	//fmt.Println("  writeToHeap/alloc:", ptr, size)

	if track && size != 0 {
		h.allocations = append(h.allocations, &allocation{ptr: ptr, length: size})
	}
	return h.WriteAtPtr(bytes, ptr, from)
}

func (h *Heap) WriteAtPtr(bytes []byte, ptr int32, from string) (int32, error) {
	data := h.memory.UnsafeData(h.store)
	copy(data[ptr:], bytes)
	return ptr, nil
}

func (h *Heap) Clear() error {
	for _, a := range h.allocations {
		//fmt.Println("  dealloc", a.ptr, a.length)
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
