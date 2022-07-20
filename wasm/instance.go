package wasm

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
)

type Instance struct {
	//store        *wasmer.Store
	inputStores  []state.Reader
	outputStore  *state.Store
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	valueType string

	clock *pbsubstreams.Clock

	args         []uint64 // to the `entrypoint` function
	returnValue  []byte
	panicError   *PanicError
	functionName string
	moduleName   string

	Logs          []string
	LogsByteCount uint64
	Module        *Module
}

func (i *Instance) Execute(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		HeapStatsInstance.addDuration("execution", time.Since(start))
	}()
	if _, err = i.Module.entrypoint.Call(ctx, i.args...); err != nil {
		if extern, ok := err.(*sys.ExitError); ok {
			if extern.ExitCode() == 0 {
				return nil
			}
		}
		if i.panicError != nil {
			fmt.Println("Panic error:", i.panicError)
			return i.panicError
		}
		return fmt.Errorf("executing entrypoint %q: %w", i.functionName, err)
	}

	return nil
}

func (i *Instance) ExecuteWithArgs(ctx context.Context, args ...uint64) (err error) {
	if _, err = i.Module.entrypoint.Call(ctx, args...); err != nil {
		if extern, ok := err.(*sys.ExitError); ok {
			if extern.ExitCode() == 0 {
				return nil
			}
		}

		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing with args entrypoint %q: %w", i.functionName, err)
	}
	return nil
}

func (i *Instance) WriteOutputToHeap(ctx context.Context, memory api.Memory, outputPtr uint32, value []byte, from string) error {
	valuePtr, err := i.Module.Heap.WriteAndTrack(ctx, memory, value, false, from+":WriteOutputToHeap1")
	if err != nil {
		return fmt.Errorf("writting value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], valuePtr)
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = i.Module.Heap.WriteAtPtr(ctx, memory, returnValue, outputPtr, from+":WriteOutputToHeap2")
	if err != nil {
		return fmt.Errorf("writing response at valuePtr %d: %w", valuePtr, err)
	}

	return nil
}

func (i *Instance) Err() error {
	return i.panicError
}

func (i *Instance) Output() []byte {
	return i.returnValue
}

func (i *Instance) SetOutputStore(store *state.Store) {
	i.outputStore = store
}

const maxLogByteCount = 128 * 1024 // 128 KiB

func (i *Instance) ReachedLogsMaxByteCount() bool {
	return i.LogsByteCount >= maxLogByteCount
}
