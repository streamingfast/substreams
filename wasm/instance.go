package wasm

import (
	"encoding/binary"
	"fmt"

	"github.com/streamingfast/substreams/storage/store"

	"github.com/bytecodealliance/wasmtime-go"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Instance struct {
	inputStores  []store.Reader
	outputStore  store.Store
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	valueType string

	clock *pbsubstreams.Clock

	args        []interface{} // to the `entrypoint` function
	returnValue []byte
	panicError  *PanicError

	Logs           []string
	LogsByteCount  uint64
	ExecutionStack []string
	Module         *Module
	entrypoint     *wasmtime.Func
}

func (i *Instance) Execute() (err error) {
	if _, err = i.entrypoint.Call(i.Module.wasmStore, i.args...); err != nil {
		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing module %q: %w", i.Module.name, err)
	}
	return nil
}

func (i *Instance) ExecuteWithArgs(args ...interface{}) (err error) {
	if _, err = i.entrypoint.Call(i.Module.wasmStore, args...); err != nil {
		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing module with args %q: %w", i.Module.name, err)
	}
	return nil
}

func (i *Instance) WriteOutputToHeap(outputPtr int32, value []byte, from string) error {
	valuePtr, err := i.Module.Heap.WriteAndTrack(value, false, from+":WriteOutputToHeap1")
	if err != nil {
		return fmt.Errorf("writting value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], uint32(valuePtr))
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = i.Module.Heap.WriteAtPtr(returnValue, outputPtr, from+":WriteOutputToHeap2")
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

func (i *Instance) SetOutputStore(store store.Store) {
	i.outputStore = store
}

const maxLogByteCount = 128 * 1024 // 128 KiB

func (i *Instance) ReachedLogsMaxByteCount() bool {
	return i.LogsByteCount >= maxLogByteCount
}

func (i *Instance) PushExecutionStack(event string) {
	i.ExecutionStack = append(i.ExecutionStack, event)
}

func (i *Instance) Cleanup() error {
	err := i.Module.Heap.Clear()
	if err != nil {
		return fmt.Errorf("clearing heap: %w", err)
	}
	i.Module.wasmStore.GC()
	return err
}
