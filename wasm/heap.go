package wasm

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/tetratelabs/wazero/api"
)

type allocation struct {
	ptr    uint32
	length int
}

var HeapStatsInstance = NewHeapStats()

type Heap struct {
	allocations []*allocation
	allocator   api.Function
	dealloc     api.Function
}

type HeapStats map[string]time.Duration

func NewHeapStats() *HeapStats {
	return &HeapStats{}
}

func (h HeapStats) addDuration(metric string, duration time.Duration) {
	if _, ok := h[metric]; !ok {
		h[metric] = duration
	} else {
		h[metric] += duration
	}
}

func (h HeapStats) Print() {
	for k, duration := range h {
		fmt.Println(k, ":\t\t", duration)
	}
}

func NewHeap(allocator, dealloc api.Function) *Heap {
	return &Heap{
		allocator: allocator,
		dealloc:   dealloc,
	}
}
func (h *Heap) Write(ctx context.Context, memory api.Memory, bytes []byte, from string) (uint32, error) {
	return h.WriteAndTrack(ctx, memory, bytes, true, from)
}

func (h *Heap) WriteAndTrack(ctx context.Context, memory api.Memory, bytes []byte, track bool, from string) (uint32, error) {
	size := len(bytes)

	start := time.Now()
	results, err := h.allocator.Call(ctx, uint64(size))
	if err != nil {
		return 0, fmt.Errorf("allocating memory for size %d:%w", size, err)
	}
	HeapStatsInstance.addDuration("alloc", time.Since(start))

	ptr := results[0]
	if track {
		h.allocations = append(h.allocations, &allocation{ptr: uint32(ptr), length: len(bytes)})
	}
	return h.WriteAtPtr(ctx, memory, bytes, uint32(ptr), from)
}
func (h *Heap) WriteAtPtr(ctx context.Context, memory api.Memory, bytes []byte, ptr uint32, from string) (uint32, error) {
	start := time.Now()
	if !memory.Write(ctx, ptr, bytes) {
		return 0, fmt.Errorf("failed writing to memory at ptr %d", ptr)
	}
	HeapStatsInstance.addDuration("write", time.Since(start))
	return ptr, nil
}

func (h *Heap) Clear(ctx context.Context) error {

	start := time.Now()
	sort.Slice(h.allocations, func(i, j int) bool {
		return h.allocations[i].ptr < h.allocations[j].ptr
	})
	for _, a := range h.allocations {
		if _, err := h.dealloc.Call(ctx, uint64(a.ptr), uint64(a.length)); err != nil {
			return fmt.Errorf("deallocating memory at ptr %d: %w", a.ptr, err)
		}
	}
	h.allocations = nil
	HeapStatsInstance.addDuration("dealloc", time.Since(start))
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
	start := time.Now()
	data, ok := memory.Read(ctx, ptr, length)
	if !ok {
		return nil, fmt.Errorf("failed reading bytes from memory at ptr %d", ptr)
	}

	out := make([]byte, length)
	copy(out, data)
	HeapStatsInstance.addDuration("read", time.Since(start))
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
