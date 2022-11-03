package wasm

import (
	"fmt"
	"math/big"

	"github.com/streamingfast/substreams/manifest"

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
		returnStateErrorString(fmt.Sprintf("invalid store operation: %q only valid for stores with updatePolicy == 'replace'", manifest.UpdatePolicySet))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	store := m.CurrentInstance.outputStore
	store.SetBytes(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.set  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setIfNotExists(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS {
		returnStateErrorString(fmt.Sprintf("invalid store operation: %q only valid for stores with updatePolicy == 'ignore'", manifest.UpdatePolicySetIfNotExists))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	store := m.CurrentInstance.outputStore
	store.SetBytesIfNotExists(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setIfNotExists  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) append(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND {
		returnStateErrorString(fmt.Sprintf("invalid store operation: %q only valid for stores with updatePolicy == %q", manifest.UpdatePolicyAppend, manifest.UpdatePolicyAppend))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadBytes(valPtr, valLength)

	store := m.CurrentInstance.outputStore
	store.Append(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.append  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) deletePrefix(ord int64, keyPtr, keyLength int32) {
	prefix := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.DeletePrefix(uint64(ord), prefix)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.deletePrefix  %s storeDetail:%s", store.Name(), prefix, store.String()))
}

func (m *Module) addBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeBigInt {

		returnErrorString("state", fmt.Sprintf("invalid store operation: 'add_bigint' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyAdd, manifest.OutputValueTypeBigInt))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toAdd, _ := new(big.Int).SetString(value, 10)

	store := m.CurrentInstance.outputStore
	store.SumBigInt(uint64(ord), key, toAdd)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.addBigInt  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) addBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD &&
		!(m.CurrentInstance.valueType == manifest.OutputValueTypeBigDecimal || m.CurrentInstance.valueType == manifest.OutputValueTypeBigFloat) {

		returnErrorString("state", fmt.Sprintf("invalid store operation: 'add_bigdecimal' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyAdd, manifest.OutputValueTypeBigDecimal))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigDecimal's read of the kv value
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigdecimal: %w", err))
	}

	store := m.CurrentInstance.outputStore
	store.SumBigDecimal(uint64(ord), key, toAdd)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.addBigDecimal  %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) addInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy !=
		pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeInt64 {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'add_int64' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyAdd, manifest.OutputValueTypeInt64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.SumInt64(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.addInt64  %q storeDetail:%s", store.Name(), key, store.String()))

}

func (m *Module) addFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeFloat64 {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'add_float64' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyAdd, manifest.OutputValueTypeFloat64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.SumFloat64(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.addFloat64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMinInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeInt64 {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_min_int64' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMin, manifest.OutputValueTypeInt64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.SetMinInt64(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMinInt64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMinBigint(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeBigInt {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_min_bigint' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMin, manifest.OutputValueTypeBigInt))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _ := new(big.Int).SetString(value, 10)

	store := m.CurrentInstance.outputStore
	store.SetMinBigInt(uint64(ord), key, toSet)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMinBigint %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMinFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeFloat64 {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_min_float' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMin, manifest.OutputValueTypeFloat64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.SetMinFloat64(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMinFloat64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMinBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN &&
		!(m.CurrentInstance.valueType == manifest.OutputValueTypeBigDecimal || m.CurrentInstance.valueType == manifest.OutputValueTypeBigFloat) {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_min_bigdecimal' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMin, manifest.OutputValueTypeBigDecimal))
	}

	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigdecimal: %w", err))
	}

	store := m.CurrentInstance.outputStore
	store.SetMinBigDecimal(uint64(ord), key, toSet)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.set_min_bigdecimal %q storeDetail:%s", store.Name(), key, store.Name()))
}

func (m *Module) setMaxInt64(ord int64, keyPtr, keyLength int32, value int64) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeInt64 {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_max_int64' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMax, manifest.OutputValueTypeInt64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.SetMaxInt64(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMaxInt64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMaxBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeBigInt {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_max_bigint' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMax, manifest.OutputValueTypeBigInt))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _ := new(big.Int).SetString(value, 10)

	store := m.CurrentInstance.outputStore
	store.SetMaxBigInt(uint64(ord), key, toSet)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMaxBigInt %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMaxFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		m.CurrentInstance.valueType != manifest.OutputValueTypeFloat64 {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_max_float' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMax, manifest.OutputValueTypeFloat64))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)

	store := m.CurrentInstance.outputStore
	store.SetMaxFloat64(uint64(ord), key, value)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMaxFloat64 %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) setMaxBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	if m.CurrentInstance.outputStore == nil &&
		m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		!(m.CurrentInstance.valueType == manifest.OutputValueTypeBigDecimal || m.CurrentInstance.valueType == manifest.OutputValueTypeBigFloat) {

		returnStateErrorString(fmt.Sprintf("invalid store operation: 'set_max_bigdecimal' only valid for stores with updatePolicy == %q and valueType == %q", manifest.UpdatePolicyMax, manifest.OutputValueTypeBigDecimal))
	}
	key := m.Heap.ReadString(keyPtr, keyLength)
	value := m.Heap.ReadString(valPtr, valLength)

	toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
	if err != nil {
		returnStateError(fmt.Errorf("parsing bigdecimal: %w", err))
	}

	store := m.CurrentInstance.outputStore
	store.SetMaxBigDecimal(uint64(ord), key, toSet)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.setMaxBigDecimal %q storeDetail:%s", store.Name(), key, store.String()))
}

func (m *Module) getAt(storeIndex int32, ord int64, keyPtr, keyLength, outputPtr int32) int32 {
	if int(storeIndex+1) > len(m.CurrentInstance.inputStores) {
		returnStateError(fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores)))
	}
	readStore := m.CurrentInstance.inputStores[storeIndex]
	key := m.Heap.ReadString(keyPtr, keyLength)
	value, found := readStore.GetAt(uint64(ord), key)
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.getAt %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
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
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.getFirst %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
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
	m.CurrentInstance.PushExecutionStack(fmt.Sprintf("%s.getLast %q: found:%t storeDetail:%s", readStore.Name(), key, found, readStore.String()))
	if !found {
		return 0
	}

	err := m.CurrentInstance.WriteOutputToHeap(outputPtr, value, key)
	if err != nil {
		returnStateError(fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err))

	}
	return 1
}
