package state

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var (
	updatePolicyKey     = strings.Join([]string{string([]byte{255}), "update-policy"}, "")
	valueTypeKey        = strings.Join([]string{string([]byte{255}), "value-type"}, "")
	storeNameKey        = strings.Join([]string{string([]byte{255}), "store-name"}, "")
	moduleHashKey       = strings.Join([]string{string([]byte{255}), "module-hash"}, "")
	moduleStartBlockKey = strings.Join([]string{string([]byte{255}), "module-start-block"}, "")
)

const (
	OutputValueTypeInt64    = "int64"
	OutputValueTypeFloat64  = "float64"
	OutputValueTypeBigInt   = "bigInt"
	OutputValueTypeBigFloat = "bigFloat"
	OutputValueTypeString   = "string"
)

func (b *Builder) writeMergeValues() {
	b.KV[updatePolicyKey] = []byte(strconv.Itoa(int(b.updatePolicy)))
	b.KV[valueTypeKey] = []byte(b.valueType)
	b.KV[moduleHashKey] = []byte(b.ModuleHash)
	b.KV[moduleStartBlockKey] = intToBytes(int(b.ModuleStartBlock))
	b.KV[storeNameKey] = []byte(b.Name)
}

func readMergeValues(kv map[string][]byte) (updatedKV map[string][]byte, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, moduleHash string, moduleStartBlock uint64, storeName string) {
	//mega-hack because json marshalling converts invalid UTF-8 (in our case byte 255) to Unicode replacement character U+FFFD (byte 239-191-189)  (see json package documentation)
	//here, we convert that back to byte 255
	//todo: do this in json.Unmarshal implementation of a new KV type.
	for k, v := range kv {
		bk := []byte(k)
		if bytes.HasPrefix([]byte(k), []byte{239, 191, 189}) {
			bk := bytes.Replace(bk, []byte{239, 191, 189}, []byte{255}, 1)
			delete(kv, k)
			kv[string(bk)] = v
		}
	}

	///TODO(colin): do not delete the special keys.

	if updatePolicyBytes, ok := kv[updatePolicyKey]; ok {
		updatePolicyInt, err := strconv.Atoi(string(updatePolicyBytes))
		if err != nil {
			panic(fmt.Errorf("parsing update policy value: %w", err))
		}
		updatePolicy = pbsubstreams.Module_KindStore_UpdatePolicy(int32(updatePolicyInt))
		delete(kv, updatePolicyKey)
	}

	if valueTypeBytes, ok := kv[valueTypeKey]; ok {
		valueType = string(valueTypeBytes)
		delete(kv, valueTypeKey)
	}

	if moduleHashBytes, ok := kv[moduleHashKey]; ok {
		moduleHash = string(moduleHashBytes)
		delete(kv, moduleHashKey)
	}

	if moduleStartBlockBytes, ok := kv[moduleStartBlockKey]; ok {
		moduleStartBlock = uint64(bytesToInt(moduleStartBlockBytes))
		delete(kv, moduleStartBlockKey)
	}

	if storeNameBytes, ok := kv[storeNameKey]; ok {
		storeName = string(storeNameBytes)
		delete(kv, storeNameKey)
	}

	updatedKV = kv

	return
}

func (b *Builder) readMergeValues() {
	b.KV, b.updatePolicy, b.valueType, b.ModuleHash, b.ModuleStartBlock, b.Name = readMergeValues(b.KV)
}

func (b *Builder) Merge(previous *Builder) error {
	next := b

	if next.updatePolicy != previous.updatePolicy {
		return fmt.Errorf("incompatible update policies: policy %q cannot merge policy %q", next.updatePolicy, previous.updatePolicy)
	}

	if next.valueType != previous.valueType {
		return fmt.Errorf("incompatible value types: cannot merge %q and %q", next.valueType, previous.valueType)
	}

	for _, p := range next.DeletedPrefixes {
		previous.DeletePrefix(previous.lastOrdinal, p)
	}

	switch next.updatePolicy {
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_REPLACE:
		for k, v := range previous.KV {
			if _, found := next.KV[k]; !found {
				next.KV[k] = v
			}
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_IGNORE:
		for k, v := range previous.KV {
			next.KV[k] = v
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SUM:
		// check valueType to do the right thing
		switch next.valueType {
		case OutputValueTypeInt64:
			sum := func(a, b uint64) uint64 {
				return a + b
			}
			for k, v := range previous.KV {
				v0b, fv0 := next.KV[k]
				v0 := foundOrZeroUint64(v0b, fv0)
				v1 := foundOrZeroUint64(v, true)
				next.KV[k] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case OutputValueTypeFloat64:
			sum := func(a, b float64) float64 {
				return a + b
			}
			for k, v := range previous.KV {
				v0b, fv0 := next.KV[k]
				v0 := foundOrZeroFloat(v0b, fv0)
				v1 := foundOrZeroFloat(v, true)
				next.KV[k] = []byte(floatToStr(sum(v0, v1)))
			}
		case OutputValueTypeBigInt:
			sum := func(a, b *big.Int) *big.Int {
				return bi().Add(a, b)
			}
			for k, v := range previous.KV {
				v0b, fv0 := next.KV[k]
				v0 := foundOrZeroBigInt(v0b, fv0)
				v1 := foundOrZeroBigInt(v, true)
				next.KV[k] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			sum := func(a, b *big.Float) *big.Float {
				return bf().Add(a, b).SetPrec(100)
			}
			for k, v := range previous.KV {
				v0b, fv0 := next.KV[k]
				v0 := foundOrZeroBigFloat(v0b, fv0)
				v1 := foundOrZeroBigFloat(v, true)
				next.KV[k] = []byte(bigFloatToStr(sum(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", next.updatePolicy, next.valueType)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX:
		switch next.valueType {
		case OutputValueTypeInt64:
			max := func(a, b uint64) uint64 {
				if a >= b {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroUint64(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroUint64(v, true)

				next.KV[k] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case OutputValueTypeFloat64:
			min := func(a, b float64) float64 {
				if a < b {
					return b
				}
				return a
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroFloat(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(floatToStr(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				next.KV[k] = []byte(floatToStr(min(v0, v1)))
			}
		case OutputValueTypeBigInt:
			max := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroBigInt(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigInt(v, true)

				next.KV[k] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			max := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroBigFloat(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(bigFloatToStr(v1))
					continue
				}
				v0 := foundOrZeroBigFloat(v, true)

				next.KV[k] = []byte(bigFloatToStr(max(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", next.updatePolicy, next.valueType)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN:
		switch next.valueType {
		case OutputValueTypeInt64:
			min := func(a, b uint64) uint64 {
				if a <= b {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroUint64(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroUint64(v, true)

				next.KV[k] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case OutputValueTypeFloat64:
			min := func(a, b float64) float64 {
				if a < b {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroFloat(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(floatToStr(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				next.KV[k] = []byte(floatToStr(min(v0, v1)))
			}
		case OutputValueTypeBigInt:
			min := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroBigInt(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigInt(v, true)

				next.KV[k] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			min := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroBigFloat(v, true)
				v, found := next.KV[k]
				if !found {
					next.KV[k] = []byte(bigFloatToStr(v1))
					continue
				}
				v0 := foundOrZeroBigFloat(v, true)

				next.KV[k] = []byte(bigFloatToStr(min(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", next.updatePolicy, next.valueType)
		}
	default:
		return fmt.Errorf("update policy %q not supported", next.updatePolicy) // should have been validated already
	}

	next.partialMode = previous.partialMode
	if next.partialMode {
		next.partialStartBlock = previous.partialStartBlock
	}

	return nil
}

func foundOrZeroUint64(in []byte, found bool) uint64 {
	if !found {
		return 0
	}
	val, err := strconv.ParseInt(string(in), 10, 64)
	if err != nil {
		return 0
	}
	return uint64(val)
}

func foundOrZeroBigFloat(in []byte, found bool) *big.Float {
	if !found {
		return bf()
	}
	return bytesToBigFloat(in)
}

func foundOrZeroBigInt(in []byte, found bool) *big.Int {
	if !found {
		return bi()
	}
	return bytesToBigInt(in)
}

func foundOrZeroFloat(in []byte, found bool) float64 {
	if !found {
		return float64(0)
	}

	f, err := strconv.ParseFloat(string(in), 64)
	if err != nil {
		return float64(0)
	}
	return f
}

func strToBigFloat(in string) *big.Float {
	newFloat, _, err := big.ParseFloat(in, 10, 100, big.ToNearestEven)
	if err != nil {
		panic(fmt.Sprintf("cannot load float %q: %s", in, err))
	}
	return newFloat.SetPrec(100)
}

func strToFloat(in string) float64 {
	newFloat, _, err := big.ParseFloat(in, 10, 100, big.ToNearestEven)
	if err != nil {
		panic(fmt.Sprintf("cannot load float %q: %s", in, err))
	}
	f, _ := newFloat.SetPrec(100).Float64()
	return f
}

func strToBigInt(in string) *big.Int {
	i64, err := strconv.ParseInt(in, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("cannot load int %q: %s", in, err))
	}
	return big.NewInt(i64)
}

func bytesToBigFloat(in []byte) *big.Float {
	return strToBigFloat(string(in))
}

func bytesToBigInt(in []byte) *big.Int {
	return strToBigInt(string(in))
}

func floatToStr(f float64) string {
	return big.NewFloat(f).Text('g', -1)
}

func floatToBytes(f float64) []byte {
	return []byte(floatToStr(f))
}

func intToBytes(i int) []byte {
	return []byte(strconv.Itoa(i))
}

func bytesToInt(b []byte) int {
	i, err := strconv.Atoi(string(b))
	if err != nil {
		panic(fmt.Sprintf("cannot convert string %s to int: %s", string(b), err.Error()))
	}
	return i
}

func bigFloatToStr(f *big.Float) string {
	return f.Text('g', -1)
}

func bigFloatToBytes(f *big.Float) []byte {
	return []byte(bigFloatToStr(f))
}

var bf = func() *big.Float { return new(big.Float).SetPrec(100) }
var bi = func() *big.Int { return new(big.Int) }
