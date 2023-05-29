package wasm

import (
	"fmt"
	"math/big"

	"github.com/streamingfast/substreams/bigdecimal"
	"github.com/streamingfast/substreams/manifest"
)

func returnStateErrorString(cause string) {
	returnErrorString("state", cause)
}
func returnStateError(cause error) {
	returnError("state", cause)
}

func (m *Instance) set(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidSetStore() {
		returnStateErrorString(fmt.Sprintf("invalid store %q  operation: %q only valid for stores with updatePolicy == 'replace'", m.CurrentCall.instance.name, manifest.UpdatePolicySet))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	store := m.CurrentCall.outputStore
	store.SetBytes(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.set  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setIfNotExists(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidSetIfNotExists() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: %q only valid for stores with updatePolicy == 'ignore'", m.CurrentCall.instance.name, manifest.UpdatePolicySetIfNotExists))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	store := m.CurrentCall.outputStore
	store.SetBytesIfNotExists(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setIfNotExists  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) append(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidAppendStore() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: %q only valid for stores with updatePolicy == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyAppend, manifest.UpdatePolicyAppend))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	store := m.CurrentCall.outputStore
	err := store.Append(uint64(ord), key, value)
	if err != nil {
		returnStateError(fmt.Errorf("appending to store: %w", err))
	}
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.append  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) deletePrefix(ord int64, keyPtr, keyLength int32) {
	prefix := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.DeletePrefix(uint64(ord), prefix)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.deletePrefix  %s storeDetail:%s", store.Name(), prefix, store.String()))
}

func (m *Instance) addBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidAddBigIntStore() {
		returnErrorString("state", fmt.Sprintf("invalid store %q operation: 'add_bigint' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyAdd, manifest.OutputValueTypeBigInt))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toAdd, _ := new(big.Int).SetString(value, 10)

	store := m.CurrentCall.outputStore
	store.SumBigInt(uint64(ord), key, toAdd)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.addBigInt  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) addBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidAddBigDecimalStore() {
		returnErrorString("state", fmt.Sprintf("invalid store %q operation: 'add_bigdecimal' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyAdd, manifest.OutputValueTypeBigDecimal))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toAdd, err := bigdecimal.NewFromString(string(value))
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigdecimal: %w", err))
	}

	store := m.CurrentCall.outputStore
	store.SumBigDecimal(uint64(ord), key, toAdd)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.addBigDecimal  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) addInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if !m.CurrentCall.IsValidAddInt64Store() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'add_int64' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyAdd, manifest.OutputValueTypeInt64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.SumInt64(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.addInt64  %q storeDetail:%s", store.Name(), key, store.String()))

}

func (m *Instance) addFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if !m.CurrentCall.IsValidAddFloat64Store() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'add_float64' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyAdd, manifest.OutputValueTypeFloat64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.SumFloat64(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.addFloat64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMinInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if !m.CurrentCall.IsValidSetMinInt64Store() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_min_int64' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMin, manifest.OutputValueTypeInt64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.SetMinInt64(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMinInt64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMinBigint(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidSetMinBigIntStore() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_min_bigint' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMin, manifest.OutputValueTypeBigInt))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _ := new(big.Int).SetString(value, 10)

	store := m.CurrentCall.outputStore
	store.SetMinBigInt(uint64(ord), key, toSet)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMinBigint %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMinFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if !m.CurrentCall.IsValidSetMinFloat64Store() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_min_float' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMin, manifest.OutputValueTypeFloat64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.SetMinFloat64(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMinFloat64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMinBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidSetMinBigDecimalStore() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_min_bigdecimal' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMin, manifest.OutputValueTypeBigDecimal))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigdecimal: %w", err))
	}

	store := m.CurrentCall.outputStore
	store.SetMinBigDecimal(uint64(ord), key, toSet)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.set_min_bigdecimal %q storeDetail:%s", store.Name(), key, store.Name()))
}

func (m *Instance) setMaxInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if !m.CurrentCall.IsValidSetMaxInt64Store() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_max_int64' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMax, manifest.OutputValueTypeInt64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.SetMaxInt64(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMaxInt64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMaxBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidSetMaxBigIntStore() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_max_bigint' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMax, manifest.OutputValueTypeBigInt))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _ := new(big.Int).SetString(value, 10)

	store := m.CurrentCall.outputStore
	store.SetMaxBigInt(uint64(ord), key, toSet)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMaxBigInt %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMaxFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if !m.CurrentCall.IsValidSetMaxFloat64Store() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_max_float' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMax, manifest.OutputValueTypeFloat64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentCall.outputStore
	store.SetMaxFloat64(uint64(ord), key, value)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMaxFloat64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) setMaxBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if !m.CurrentCall.IsValidSetMaxBigDecimalStore() {
		returnStateErrorString(fmt.Sprintf("invalid store %q operation: 'set_max_bigdecimal' only valid for stores with updatePolicy == %q and valueType == %q", m.CurrentCall.instance.name, manifest.UpdatePolicyMax, manifest.OutputValueTypeBigDecimal))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigdecimal: %w", err))
	}

	store := m.CurrentCall.outputStore
	store.SetMaxBigDecimal(uint64(ord), key, toSet)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.setMaxBigDecimal %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Instance) getAt(storeIndex int32, ord int64, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex+1) > len(m.CurrentCall.inputStores) {
		returnStateError(fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentCall.inputStores)))
	}
	readStore := m.CurrentCall.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetAt(uint64(ord), key)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.getAt %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}

	err := m.CurrentCall.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))
	}
	return 1
}

func (m *Instance) hasAt(storeIndex int32, ord int64, keyPtr, keyLength int32) int32 {
	if int(storeIndex+1) > len(m.CurrentCall.inputStores) {
		returnStateError(fmt.Errorf("'has_at' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentCall.inputStores)))
	}
	readStore := m.CurrentCall.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	found := readStore.HasAt(uint64(ord), key)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.hasAt %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}
	return 1
}

func (m *Instance) getFirst(storeIndex int32, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex)+1 > len(m.CurrentCall.inputStores) {
		returnStateError(fmt.Errorf("'get_first' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentCall.inputStores)))
	}
	readStore := m.CurrentCall.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetFirst(key)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.getFirst %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}
	err := m.CurrentCall.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))
	}
	return 1
}

func (m *Instance) hasFirst(storeIndex int32, keyPtr, keyLength int32) int32 {
	if int(storeIndex)+1 > len(m.CurrentCall.inputStores) {
		returnStateError(fmt.Errorf("'has_first' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentCall.inputStores)))
	}
	readStore := m.CurrentCall.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	found := readStore.HasFirst(key)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.hasFirst %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}
	return 1
}

func (m *Instance) getLast(storeIndex int32, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex)+1 > len(m.CurrentCall.inputStores) {
		returnStateError(fmt.Errorf("'get_last' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentCall.inputStores)))
	}

	readStore := m.CurrentCall.inputStores[storeIndex]

	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetLast(key)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.getLast %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}

	err := m.CurrentCall.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))

	}
	return 1
}

func (m *Instance) hasLast(storeIndex int32, keyPtr, keyLength int32) int32 {
	if int(storeIndex)+1 > len(m.CurrentCall.inputStores) {
		returnStateError(fmt.Errorf("'has_last' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentCall.inputStores)))
	}
	readStore := m.CurrentCall.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	found := readStore.HasLast(key)
	m.CurrentCall.PushExecutionStack(fmt.Sprintf("%s.hasLast %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}
	return 1
}
