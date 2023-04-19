package wasm

import (
	"encoding/binary"
	"fmt"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/storage/store"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Call struct {
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
	instance       *Instance
	entrypoint     *wasmtime.Func
}

func (c *Call) Execute() (err error) {
	if _, err = c.entrypoint.Call(c.instance.wasmStore, c.args...); err != nil {
		if c.panicError != nil {
			return c.panicError
		}
		return fmt.Errorf("executing module %q: %w", c.instance.name, err)
	}
	return nil
}

func (c *Call) ExecuteWithArgs(args ...interface{}) (err error) {
	if _, err = c.entrypoint.Call(c.instance.wasmStore, args...); err != nil {
		if c.panicError != nil {
			return c.panicError
		}
		return fmt.Errorf("executing module with args %q: %w", c.instance.name, err)
	}
	return nil
}

func (c *Call) WriteOutputToHeap(outputPtr int32, value []byte, from string) error {
	valuePtr, err := c.instance.Heap.WriteAndTrack(value, false, from+":WriteOutputToHeap1")
	if err != nil {
		return fmt.Errorf("writting value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], uint32(valuePtr))
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = c.instance.Heap.WriteAtPtr(returnValue, outputPtr, from+":WriteOutputToHeap2")
	if err != nil {
		return fmt.Errorf("writing response at valuePtr %d: %w", valuePtr, err)
	}

	return nil
}

func (c *Call) Err() error {
	return c.panicError
}

func (c *Call) Output() []byte {
	return c.returnValue
}

func (c *Call) SetOutputStore(store store.Store) {
	c.outputStore = store
}

const maxLogByteCount = 128 * 1024 // 128 KiB

func (c *Call) ReachedLogsMaxByteCount() bool {
	return c.LogsByteCount >= maxLogByteCount
}

func (c *Call) PushExecutionStack(event string) {
	c.ExecutionStack = append(c.ExecutionStack, event)
}

func (c *Call) Cleanup() error {
	err := c.instance.Heap.Clear()
	if err != nil {
		return fmt.Errorf("clearing heap: %w", err)
	}
	c.instance.wasmStore.GC()
	return err
}

func (c *Call) IsValidSetStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_SET
}

func (c *Call) IsValidSetIfNotExists() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS
}

func (c *Call) IsValidAppendStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND
}

func (c *Call) IsValidAddBigIntStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && c.valueType == manifest.OutputValueTypeBigInt
}

func (c *Call) IsValidAddBigDecimalStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD &&
		(c.valueType == manifest.OutputValueTypeBigDecimal || c.valueType == manifest.OutputValueTypeBigFloat)
}

func (c *Call) IsValidAddInt64Store() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && c.valueType == manifest.OutputValueTypeInt64
}

func (c *Call) IsValidAddFloat64Store() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && c.valueType == manifest.OutputValueTypeFloat64
}

func (c *Call) IsValidSetMinInt64Store() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && c.valueType == manifest.OutputValueTypeInt64
}

func (c *Call) IsValidSetMinBigIntStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && c.valueType == manifest.OutputValueTypeBigInt
}

func (c *Call) IsValidSetMinFloat64Store() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && c.valueType == manifest.OutputValueTypeFloat64
}

func (c *Call) IsValidSetMinBigDecimalStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN &&
		(c.valueType == manifest.OutputValueTypeBigDecimal || c.valueType == manifest.OutputValueTypeBigFloat)
}

func (c *Call) IsValidSetMaxInt64Store() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		c.valueType == manifest.OutputValueTypeInt64
}

func (c *Call) IsValidSetMaxBigIntStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		c.valueType == manifest.OutputValueTypeBigInt
}

func (c *Call) IsValidSetMaxFloat64Store() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		c.valueType == manifest.OutputValueTypeFloat64
}

func (c *Call) IsValidSetMaxBigDecimalStore() bool {
	return c.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		(c.valueType == manifest.OutputValueTypeBigDecimal || c.valueType == manifest.OutputValueTypeBigFloat)
}
