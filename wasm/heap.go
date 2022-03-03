package wasm

import (
	"fmt"

	"github.com/wasmerio/wasmer-go/wasmer"
)

type Heap struct {
	memory    *wasmer.Memory
	allocator wasmer.NativeFunction
}

func NewHeap(memory *wasmer.Memory, allocator wasmer.NativeFunction) *Heap {
	return &Heap{
		memory:    memory,
		allocator: allocator,
	}
}

func (h *Heap) Write(bytes []byte) (int32, error) {
	size := len(bytes)

	allocation, err := h.allocator(size)
	if err != nil {
		return 0, fmt.Errorf("allocating memory for size %d:%w", size, err)
	}

	ptr := allocation.(int32)

	memoryData := h.memory.Data()
	copy(memoryData[ptr:], bytes)

	return ptr, nil
}

func (h *Heap) ReadString(offset int32, length int32) (string, error) {
	bytes, err := h.ReadBytes(offset, length)
	if err != nil {
		return "", fmt.Errorf("read bytes: %w", err)
	}
	return string(bytes), nil
}

func (h *Heap) ReadBytes(offset int32, length int32) ([]byte, error) {
	bytes := h.memory.Data()
	if offset < 0 {
		return nil, fmt.Errorf("offset %d env must be positive", offset)
	}

	if offset >= int32(len(bytes)) {
		return nil, fmt.Errorf("offset %d env out of memory bounds ending at %d env", offset, len(bytes))
	}

	end := offset + length
	if end > int32(len(bytes)) {
		return nil, fmt.Errorf("end %d env out of memory bounds ending at %d env", end, len(bytes))
	}

	out := make([]byte, length)
	copy(out, bytes[offset:offset+length])

	return out, nil
}

func (h *Heap) PrintMem() {
	data := h.memory.Data()
	for i, datum := range data {
		if i > 1024 {
			if datum == 0 {
				continue
			}
		}
		fmt.Print(datum, ", ")
	}
	fmt.Print("\n")
}
