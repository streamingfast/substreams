package wasm

import (
	"context"
	"fmt"
	"math/big"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/tetratelabs/wazero/api"
)

func returnStateErrorString(cause string) {
	returnErrorString("state", cause)
}
func returnStateError(cause error) {
	returnError("state", cause)
}

func (m *Module) set(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET {
		returnStateErrorString("invalid store operation: 'set' only valid for stores with updatePolicy == 'replace'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, err := m.CurrentInstance.heap.ReadBytes(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	m.CurrentInstance.outputStore.SetBytes(uint64(ord), key, value)
}

func (m *Module) setIfNotExists(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS {
		returnStateErrorString("invalid store operation: 'set_if_not_exists' only valid for stores with updatePolicy == 'ignore'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, err := m.CurrentInstance.heap.ReadBytes(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	m.CurrentInstance.outputStore.SetBytesIfNotExists(ord, key, value)
}

func (m *Module) append(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND {
		returnStateErrorString("invalid store operation: 'append' only valid for stores with updatePolicy == 'append'")
	}

	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	value, err := m.CurrentInstance.heap.ReadBytes(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}
	m.CurrentInstance.outputStore.Append(ord, key, value)
}

func (m *Module) deletePrefix(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32) {
	prefix, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading prefix: %w", err))
	}
	m.CurrentInstance.outputStore.DeletePrefix(ord, prefix)
}

func (m *Module) addBigInt(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "bigint" {
		returnErrorString("state", "invalid store operation: 'add_bigint' only valid for stores with updatePolicy == 'add' and valueType == 'bigint'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	toAdd, _ := new(big.Int).SetString(value, 10)
	m.CurrentInstance.outputStore.SumBigInt(ord, key, toAdd)

	return
}

func (m *Module) addBigFloat(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "bigfloat" {
		returnErrorString("state", "invalid store operation: 'add_bigfloat' only valid for stores with updatePolicy == 'add' and valueType == 'bigfloat'")
	}

	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))

	}
	value, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigFloat's read of the kv value
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigfloat: %w", err))
	}

	m.CurrentInstance.outputStore.SumBigFloat(ord, key, toAdd)
}

func (m *Module) addInt64(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "int64" {
		returnStateErrorString("invalid store operation: 'add_int64' only valid for stores with updatePolicy == 'add' and valueType == 'int64'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	m.CurrentInstance.outputStore.SumInt64(ord, key, value)
}

func (m *Module) addFloat64(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32, value float64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "float64" {
		returnStateErrorString("invalid store operation: 'add_float64' only valid for stores with updatePolicy == 'add' and valueType == 'float64'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	m.CurrentInstance.outputStore.SumFloat64(ord, key, value)

}

func (m *Module) setMinInt64(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "int64" {
		returnStateErrorString("invalid store operation: 'set_min_int64' only valid for stores with updatePolicy == 'min' and valueType == 'int64'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	m.CurrentInstance.outputStore.SetMinInt64(ord, key, value)
}

func (m *Module) setMinBigint(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "bigfloat" {
		returnStateErrorString("invalid store operation: 'set_min_bigint' only valid for stores with updatePolicy == 'min' and valueType == 'bigint'")
	}

	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	toSet, _ := new(big.Int).SetString(value, 10)
	m.CurrentInstance.outputStore.SetMinBigInt(ord, key, toSet)
}

func (m *Module) setMinfloat64(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32, value float64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "float" {
		returnStateErrorString("invalid store operation: 'set_min_float' only valid for stores with updatePolicy == 'min' and valueType == 'float'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	m.CurrentInstance.outputStore.SetMinFloat64(ord, key, value)
}

func (m *Module) setMinBigfloat(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "bigint" {
		returnStateErrorString("invalid store operation: 'set_min_bigfloat' only valid for stores with updatePolicy == 'min' and valueType == 'bigfloat'")
	}

	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	m.CurrentInstance.outputStore.SetMinBigFloat(ord, key, toSet)
}

func (m *Module) setMaxInt64(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "int64" {
		returnStateErrorString("invalid store operation: 'set_max_int64' only valid for stores with updatePolicy == 'max' and valueType == 'int64'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	m.CurrentInstance.outputStore.SetMaxInt64(ord, key, value)
}

func (m *Module) setMaxBigint(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "bigint" {
		returnStateErrorString("invalid store operation: 'set_max_bigint' only valid for stores with updatePolicy == 'max' and valueType == 'bigint'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))

	}
	value, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}

	toSet, _ := new(big.Int).SetString(value, 10)
	m.CurrentInstance.outputStore.SetMaxBigInt(ord, key, toSet)
}

func (m *Module) setMaxFloat64(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength uint32, value float64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "float" {
		returnStateErrorString("invalid store operation: 'set_max_float' only valid for stores with updatePolicy == 'max' and valueType == 'float'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}

	m.CurrentInstance.outputStore.SetMaxFloat64(ord, key, value)
}

func (m *Module) setMaxBigfloat(ctx context.Context, apiModule api.Module, ord uint64, keyPtr, keyLength, valPtr, valLength uint32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "bigint" {
		returnStateErrorString("invalid store operation: 'set_max_bigfloat' only valid for stores with updatePolicy == 'max' and valueType == 'bigfloat'")
	}
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), valPtr, valLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading bytes: %w", err))
	}
	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	m.CurrentInstance.outputStore.SetMaxBigFloat(ord, key, toSet)
}

func (m *Module) getAt(ctx context.Context, apiModule api.Module, storeIndex uint32, ord uint64, keyPtr, keyLength, outputPtr uint32) uint32 {
	if int(storeIndex+1) > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}
	readStore := m.CurrentInstance.inputStores[storeIndex]
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, found := readStore.GetAt(ord, key)
	if !found {
		return 0
	}

	err = m.CurrentInstance.WriteOutputToHeap(ctx, apiModule.Memory(), outputPtr, value)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))
	}
	return 1
}

func (m *Module) getFirst(ctx context.Context, apiModule api.Module, storeIndex uint32, keyPtr, keyLength, outputPtr uint32) uint32 {
	if int(storeIndex)+1 > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_first' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}
	readStore := m.CurrentInstance.inputStores[storeIndex]
	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, found := readStore.GetFirst(key)
	if !found {
		return 0
	}
	err = m.CurrentInstance.WriteOutputToHeap(ctx, apiModule.Memory(), outputPtr, value)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))
	}
	return 1
}

func (m *Module) getLast(ctx context.Context, apiModule api.Module, storeIndex uint32, keyPtr, keyLength, outputPtr uint32) uint32 {
	if int(storeIndex)+1 > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_last' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}

	readStore := m.CurrentInstance.inputStores[storeIndex]

	key, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), keyPtr, keyLength)
	if err != nil {
		returnStateError(fmt.Errorf("reading string: %w", err))
	}
	value, found := readStore.GetLast(key)
	if !found {
		return 0
	}
	err = m.CurrentInstance.WriteOutputToHeap(ctx, apiModule.Memory(), outputPtr, value)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))

	}
	return 1
}
