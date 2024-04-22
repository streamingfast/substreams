package wasm

import (
	"fmt"
	"math/big"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shopspring/decimal"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
)

type Call struct {
	Clock      *pbsubstreams.Clock // Used by WASM extensions
	ModuleName string
	Entrypoint string

	inputStores  []store.Reader
	outputStore  store.Store
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	valueType string

	returnValue []byte
	panicError  *PanicError

	Logs           []string
	LogsByteCount  uint64
	ExecutionStack []string
	stats          *metrics.Stats
}

func NewCall(clock *pbsubstreams.Clock, moduleName string, entrypoint string, stats *metrics.Stats, arguments []Argument) *Call {
	call := &Call{
		Clock:      clock,
		ModuleName: moduleName,
		Entrypoint: entrypoint,
		stats:      stats,
	}

	for _, input := range arguments {
		switch v := input.(type) {
		case *StoreWriterOutput:
			v.Store.Reset()
			call.outputStore = v.Store
			call.updatePolicy = v.UpdatePolicy
			call.valueType = v.ValueType
		case *StoreReaderInput:
			call.inputStores = append(call.inputStores, v.Store)
		case ValueArgument:
			// Handled in ÈxecuteNewCall()
		default:
			panic("unknown wasm argument type")
		}
	}

	return call
}

//func (m *Module) NewCall(clock *pbsubstreams.Clock, moduleName string, entrypoint string, arguments []Argument) (*Call, error) {
// FIXME: that's to prevent calls when context was closed, protect in the caller?
//if m.isClosed {
//	panic("module is closed")
//}

// FIXME: Replace by `context.Context`, and should speed up execution.
//if i.registry.maxFuel != 0 {
//	if remaining, _ := i.wasmStore.ConsumeFuel(i.registry.maxFuel); remaining != 0 {
//		i.wasmStore.ConsumeFuel(remaining) // don't accumulate fuel from previous executions
//	}
//	i.wasmStore.AddFuel(i.registry.maxFuel)
//}
//}

func (c *Call) Err() error {
	if c.panicError != nil {
		return c.panicError
	}
	return nil
}

func (c *Call) Output() []byte {
	return c.returnValue
}
func (c *Call) SetReturnValue(msg []byte) {
	c.returnValue = make([]byte, len(msg))
	copy(c.returnValue, msg)
}

func (c *Call) SetPanicError(message string, filename string, lineNo int, colNo int) {
	c.panicError = NewPanicError(message, filename, lineNo, colNo)
}

func (c *Call) AppendLog(message string) {
	// len(<string>) in Go count number of bytes and not characters, so we are good here
	if len(message) > MaxLogByteCount {
		panic(fmt.Errorf("message to log is too big, size is %s, max is %s", humanize.IBytes(uint64(len(message))), humanize.IBytes(uint64(MaxLogByteCount))))
	}
	c.LogsByteCount += uint64(len(message))
	if !c.ReachedLogsMaxByteCount() {
		c.Logs = append(c.Logs, message)
		c.ExecutionStack = append(c.ExecutionStack, fmt.Sprintf("log: %s", message))
	}
}

func (c *Call) SetOutputStore(store store.Store) {
	c.outputStore = store
}

const MaxLogByteCount = 128 * 1024 // 128 KiB

func (c *Call) ReachedLogsMaxByteCount() bool {
	return c.LogsByteCount >= MaxLogByteCount
}

func (c *Call) DoSet(ord uint64, key string, value []byte) {
	now := time.Now()
	c.validateSimple("set", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, key)
	c.outputStore.SetBytes(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetIfNotExists(ord uint64, key string, value []byte) {
	now := time.Now()
	c.validateSimple("set_if_not_exists", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, key)
	c.outputStore.SetBytesIfNotExists(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAppend(ord uint64, key string, value []byte) {
	now := time.Now()
	c.validateSimple("append", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, key)
	c.outputStore.Append(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoDeletePrefix(ord uint64, prefix string) {
	now := time.Now()
	c.traceStateWrites("delete_prefix", prefix)
	c.outputStore.DeletePrefix(ord, prefix)
	c.stats.RecordModuleWasmStoreDeletePrefix(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithValueType("add_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint", key)

	toAdd, _ := new(big.Int).SetString(value, 10)
	c.outputStore.SumBigInt(ord, key, toAdd)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithTwoValueTypes("add_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigdecimal", "bigfloat", key)

	toAdd, err := decimal.NewFromString(string(value))
	if err != nil {
		c.ReturnError(fmt.Errorf("parsing bigdecimal: %w", err))
	}
	c.outputStore.SumBigDecimal(ord, key, toAdd.Truncate(34))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddInt64(ord uint64, key string, value int64) {
	now := time.Now()
	c.validateWithValueType("add_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "int64", key)
	c.outputStore.SumInt64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddFloat64(ord uint64, key string, value float64) {
	now := time.Now()
	c.validateWithValueType("add_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64", key)
	c.outputStore.SumFloat64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinInt64(ord uint64, key string, value int64) {
	now := time.Now()
	c.validateWithValueType("set_min_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64", key)
	c.outputStore.SetMinInt64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithValueType("set_min_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigint", key)
	toSet, _ := new(big.Int).SetString(value, 10)
	c.outputStore.SetMinBigInt(ord, key, toSet)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinFloat64(ord uint64, key string, value float64) {
	now := time.Now()
	c.validateWithValueType("set_min_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "float64", key)
	c.outputStore.SetMinFloat64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithTwoValueTypes("set_min_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigdecimal", "bigfloat", key)
	toAdd, err := decimal.NewFromString(value)
	if err != nil {
		c.ReturnError(fmt.Errorf("parsing bigdecimal: %w", err))
	}
	c.outputStore.SetMinBigDecimal(ord, key, toAdd.Truncate(34))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxInt64(ord uint64, key string, value int64) {
	now := time.Now()
	c.validateWithValueType("set_max_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64", key)
	c.outputStore.SetMaxInt64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithValueType("set_max_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint", key)
	toSet, _ := new(big.Int).SetString(value, 10)
	c.outputStore.SetMaxBigInt(ord, key, toSet)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxFloat64(ord uint64, key string, value float64) {
	now := time.Now()
	c.validateWithValueType("set_max_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64", key)
	c.outputStore.SetMaxFloat64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithTwoValueTypes("set_max_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal", "bigfloat", key)
	toAdd, err := decimal.NewFromString(value)
	if err != nil {
		c.ReturnError(fmt.Errorf("parsing bigdecimal: %w", err))
	}
	c.outputStore.SetMaxBigDecimal(ord, key, toAdd.Truncate(34))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}

func (c *Call) DoGetAt(storeIndex int, ord uint64, key string) (value []byte, found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "get_at")
	readStore := c.inputStores[storeIndex]
	c.traceStateReads("get_at", storeIndex, found, key)
	return readStore.GetAt(ord, key)
}

func (c *Call) DoHasAt(storeIndex int, ord uint64, key string) (found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "has_at")
	readStore := c.inputStores[storeIndex]
	c.traceStateReads("has_at", storeIndex, found, key)
	return readStore.HasAt(ord, key)
}

func (c *Call) DoGetFirst(storeIndex int, key string) (value []byte, found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "get_first")
	readStore := c.inputStores[storeIndex]
	c.traceStateReads("get_first", storeIndex, found, key)
	return readStore.GetFirst(key)
}

func (c *Call) DoHasFirst(storeIndex int, key string) (found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "has_first")
	readStore := c.inputStores[storeIndex]
	c.traceStateReads("has_first", storeIndex, found, key)
	return readStore.HasFirst(key)
}

func (c *Call) DoGetLast(storeIndex int, key string) (value []byte, found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "get_last")
	readStore := c.inputStores[storeIndex]
	c.traceStateReads("get_last", storeIndex, found, key)
	return readStore.GetLast(key)
}

func (c *Call) DoHasLast(storeIndex int, key string) (found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "has_last")
	readStore := c.inputStores[storeIndex]
	c.traceStateReads("has_last", storeIndex, found, key)
	return readStore.HasLast(key)
}

func (c *Call) validateStoreIndex(storeIndex int, stateFunc string) {
	if storeIndex+1 > len(c.inputStores) {
		c.ReturnError(fmt.Errorf("%q failed: invalid store index %d, %d stores declared", stateFunc, storeIndex, len(c.inputStores)))
	}
}

func (c *Call) validateSimple(stateFunc string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, key string) {
	if c.updatePolicy != updatePolicy {
		c.returnInvalidPolicy(stateFunc, fmt.Sprintf(`updatePolicy == %q`, policyMap[updatePolicy]))
	}
	c.traceStateWrites(stateFunc, key)
}

func (c *Call) validateWithValueType(stateFunc string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, key string) {
	if c.updatePolicy != updatePolicy || c.valueType != valueType {
		c.returnInvalidPolicy(stateFunc, fmt.Sprintf(`updatePolicy == %q and valueType == %q`, policyMap[updatePolicy], valueType))
	}
	c.traceStateWrites(stateFunc, key)
}

func (c *Call) validateWithTwoValueTypes(stateFunc string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType1, valueType2 string, key string) {
	if c.updatePolicy != updatePolicy || (c.valueType != valueType1 && c.valueType != valueType2) {
		c.returnInvalidPolicy(stateFunc, fmt.Sprintf(`updatePolicy == %q and valueType == %q`, policyMap[updatePolicy], valueType1))
	}
	c.traceStateWrites(stateFunc, key)
}

func (c *Call) traceStateWrites(stateFunc, key string) {
	store := c.outputStore
	var line string
	if store == nil {
		line = fmt.Sprintf("%s key: %q", stateFunc, key)
	} else {
		line = fmt.Sprintf("%s::%s key: %q, store details: %s", store.Name(), stateFunc, key, store.String())
	}
	c.ExecutionStack = append(c.ExecutionStack, line)
}

func (c *Call) traceStateReads(stateFunc string, storeIndex int, found bool, key string) {
	store := c.inputStores[storeIndex]
	line := fmt.Sprintf("%s::%s key: %q, found: %v, store details: %s", store.Name(), stateFunc, key, found, store.String())
	c.ExecutionStack = append(c.ExecutionStack, line)
}

func (c *Call) returnInvalidPolicy(stateFunc, policy string) {
	panic(fmt.Errorf("module %q: invalid store operation %q, only valid for stores with %s", c.ModuleName, stateFunc, policy))
}

func (c *Call) ReturnError(err error) {
	panic(fmt.Errorf("module %q: %w", c.ModuleName, err))
}

var policyMap = map[pbsubstreams.Module_KindStore_UpdatePolicy]string{
	pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET:             "unset",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_SET:               "replace",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS: "ignore",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD:               "add",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN:               "min",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX:               "max",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND:            "append",
}
