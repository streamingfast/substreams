package wasm

import (
	"fmt"
	"math/big"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func returnStateErrorString(cause string) {
	returnErrorString("state", cause)
}
func returnStateError(cause error) {
	returnError("state", cause)
}

func (m *Module) set(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET {
		returnStateErrorString("invalid store operation: 'set' only valid for stores with updatePolicy == 'replace'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	m.CurrentInstance.outputStore.SetBytes(uint64(ord), key, value)
}

func (m *Module) setIfNotExists(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS {
		returnStateErrorString("invalid store operation: 'set_if_not_exists' only valid for stores with updatePolicy == 'ignore'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	m.CurrentInstance.outputStore.SetBytesIfNotExists(uint64(ord), key, value)
}

func (m *Module) append(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND {
		returnStateErrorString("invalid store operation: 'append' only valid for stores with updatePolicy == 'append'")
	}

	key := m.Heap.ReadString(keyPtr, keyLength)

	value := m.Heap.ReadBytes(valPtr, valLength)
	m.CurrentInstance.outputStore.Append(uint64(ord), key, value)
}

func (m *Module) deletePrefix(ord int64, keyPtr, keyLength int32) {
	prefix := m.Heap.ReadString(keyPtr, keyLength)
	m.CurrentInstance.outputStore.DeletePrefix(uint64(ord), prefix)
}

func (m *Module) addBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "bigint" {
		returnErrorString("state", "invalid store operation: 'add_bigint' only valid for stores with updatePolicy == 'add' and valueType == 'bigint'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toAdd, _ := new(big.Int).SetString(value, 10)
	m.CurrentInstance.outputStore.SumBigInt(uint64(ord), key, toAdd)

	return
}

func (m *Module) addBigFloat(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "bigfloat" {
		returnErrorString("state", "invalid store operation: 'add_bigfloat' only valid for stores with updatePolicy == 'add' and valueType == 'bigfloat'")
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigFloat's read of the kv value
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigfloat: %w", err))
	}

	m.CurrentInstance.outputStore.SumBigFloat(uint64(ord), key, toAdd)
}

func (m *Module) addInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "int64" {
		returnStateErrorString("invalid store operation: 'add_int64' only valid for stores with updatePolicy == 'add' and valueType == 'int64'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	m.CurrentInstance.outputStore.SumInt64(uint64(ord), key, value)
}

func (m *Module) addFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "float64" {
		returnStateErrorString("invalid store operation: 'add_float64' only valid for stores with updatePolicy == 'add' and valueType == 'float64'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	m.CurrentInstance.outputStore.SumFloat64(uint64(ord), key, value)

}

func (m *Module) setMinInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "int64" {
		returnStateErrorString("invalid store operation: 'set_min_int64' only valid for stores with updatePolicy == 'min' and valueType == 'int64'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	m.CurrentInstance.outputStore.SetMinInt64(uint64(ord), key, value)
}

func (m *Module) setMinBigint(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "bigfloat" {
		returnStateErrorString("invalid store operation: 'set_min_bigint' only valid for stores with updatePolicy == 'min' and valueType == 'bigint'")
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _ := new(big.Int).SetString(value, 10)
	m.CurrentInstance.outputStore.SetMinBigInt(uint64(ord), key, toSet)
}

func (m *Module) setMinfloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "float" {
		returnStateErrorString("invalid store operation: 'set_min_float' only valid for stores with updatePolicy == 'min' and valueType == 'float'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	m.CurrentInstance.outputStore.SetMinFloat64(uint64(ord), key, value)
}

func (m *Module) setMinBigfloat(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "bigint" {
		returnStateErrorString("invalid store operation: 'set_min_bigfloat' only valid for stores with updatePolicy == 'min' and valueType == 'bigfloat'")
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigfloat: %w", err))
	}
	m.CurrentInstance.outputStore.SetMinBigFloat(uint64(ord), key, toSet)
}

func (m *Module) setMaxInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "int64" {
		returnStateErrorString("invalid store operation: 'set_max_int64' only valid for stores with updatePolicy == 'max' and valueType == 'int64'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	m.CurrentInstance.outputStore.SetMaxInt64(uint64(ord), key, value)
}

func (m *Module) setMaxBigint(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "bigint" {
		returnStateErrorString("invalid store operation: 'set_max_bigint' only valid for stores with updatePolicy == 'max' and valueType == 'bigint'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _ := new(big.Int).SetString(value, 10)
	m.CurrentInstance.outputStore.SetMaxBigInt(uint64(ord), key, toSet)
}

func (m *Module) setMaxFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "float" {
		returnStateErrorString("invalid store operation: 'set_max_float' only valid for stores with updatePolicy == 'max' and valueType == 'float'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	m.CurrentInstance.outputStore.SetMaxFloat64(uint64(ord), key, value)
}

func (m *Module) setMaxBigfloat(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "bigint" {
		returnStateErrorString("invalid store operation: 'set_max_bigfloat' only valid for stores with updatePolicy == 'max' and valueType == 'bigfloat'")
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigfloat: %w", err))
	}
	m.CurrentInstance.outputStore.SetMaxBigFloat(uint64(ord), key, toSet)
}

func (m *Module) getAt(storeIndex int32, ord int64, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex+1) > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}
	readStore := m.CurrentInstance.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetAt(uint64(ord), key)
	if !found {
		return 0
	}

	err := m.CurrentInstance.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))
	}
	return 1
}

func (m *Module) getFirst(storeIndex int32, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex)+1 > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_first' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}
	readStore := m.CurrentInstance.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetFirst(key)
	if !found {
		return 0
	}
	err := m.CurrentInstance.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))
	}
	return 1
}

func (m *Module) getLast(storeIndex int32, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex)+1 > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_last' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}

	readStore := m.CurrentInstance.inputStores[storeIndex]

	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetLast(key)
	if !found {
		return 0
	}

	err := m.CurrentInstance.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))

	}
	return 1
}
