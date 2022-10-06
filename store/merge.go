package store

import (
	"fmt"
	"go.uber.org/zap"
	"math/big"
	"strconv"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

const (
	OutputValueTypeInt64    = "int64"
	OutputValueTypeFloat64  = "float64"
	OutputValueTypeBigInt   = "bigint"
	OutputValueTypeBigFloat = "bigfloat"
	OutputValueTypeString   = "string"
)

// Merge nextStore _into_ `s`, where nextStore is for the next contiguous segment's store output.
func (s *KVStore) Merge(kvPartialStore *KVPartialStore) error {
	zlog.Debug("merging store", zap.Object("current_store", s), zap.Object("next_store", kvPartialStore))

	if kvPartialStore.updatePolicy != s.updatePolicy {
		return fmt.Errorf("incompatible update policies: policy %q cannot merge policy %q", s.updatePolicy, kvPartialStore.updatePolicy)
	}

	if kvPartialStore.valueType != s.valueType {
		return fmt.Errorf("incompatible value types: cannot merge %q and %q", s.valueType, kvPartialStore.valueType)
	}

	for _, prefix := range kvPartialStore.DeletedPrefixes {
		s.DeletePrefix(kvPartialStore.lastOrdinal, prefix)
	}

	intoValueTypeLower := strings.ToLower(s.valueType)

	switch s.updatePolicy {
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET:
		for k, v := range kvPartialStore.kv {
			s.kv[k] = v
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS:
		for k, v := range kvPartialStore.kv {
			if _, found := s.kv[k]; !found {
				s.kv[k] = v
			}
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND:
		for key, nextVal := range kvPartialStore.kv {
			if prevVal, found := s.kv[key]; found {
				newVal := make([]byte, len(prevVal)+len(nextVal))
				copy(newVal[0:], prevVal)
				copy(newVal[len(prevVal):], nextVal)
				s.kv[key] = newVal
			} else {
				s.kv[key] = nextVal
			}
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD:
		// check valueType to do the right thing
		switch intoValueTypeLower {
		case OutputValueTypeInt64:
			sum := func(a, b uint64) uint64 {
				return a + b
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := s.kv[k]
				v0 := foundOrZeroUint64(v0b, fv0)
				v1 := foundOrZeroUint64(v, true)
				s.kv[k] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case OutputValueTypeFloat64:
			sum := func(a, b float64) float64 {
				return a + b
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := s.kv[k]
				v0 := foundOrZeroFloat(v0b, fv0)
				v1 := foundOrZeroFloat(v, true)
				s.kv[k] = []byte(floatToStr(sum(v0, v1)))
			}
		case OutputValueTypeBigInt:
			sum := func(a, b *big.Int) *big.Int {
				return bi().Add(a, b)
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := s.kv[k]
				v0 := foundOrZeroBigInt(v0b, fv0)
				v1 := foundOrZeroBigInt(v, true)
				s.kv[k] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			sum := func(a, b *big.Float) *big.Float {
				return bf().Add(a, b).SetPrec(100)
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := s.kv[k]
				v0 := foundOrZeroBigFloat(v0b, fv0)
				v1 := foundOrZeroBigFloat(v, true)
				s.kv[k] = []byte(bigFloatToStr(sum(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", s.updatePolicy, s.valueType)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX:
		switch intoValueTypeLower {
		case OutputValueTypeInt64:
			max := func(a, b uint64) uint64 {
				if a >= b {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroUint64(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroUint64(v, true)

				s.kv[k] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case OutputValueTypeFloat64:
			max := func(a, b float64) float64 {
				if a < b {
					return b
				}
				return a
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroFloat(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(floatToStr(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				s.kv[k] = []byte(floatToStr(max(v0, v1)))
			}
		case OutputValueTypeBigInt:
			max := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigInt(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigInt(v, true)

				s.kv[k] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			max := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigFloat(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(bigFloatToStr(v1))
					continue
				}
				v0 := foundOrZeroBigFloat(v, true)

				s.kv[k] = []byte(bigFloatToStr(max(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", kvPartialStore.updatePolicy, kvPartialStore.valueType)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN:
		switch intoValueTypeLower {
		case OutputValueTypeInt64:
			min := func(a, b uint64) uint64 {
				if a <= b {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroUint64(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroUint64(v, true)

				s.kv[k] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case OutputValueTypeFloat64:
			min := func(a, b float64) float64 {
				if a < b {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroFloat(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(floatToStr(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				s.kv[k] = []byte(floatToStr(min(v0, v1)))
			}
		case OutputValueTypeBigInt:
			min := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigInt(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigInt(v, true)

				s.kv[k] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			min := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigFloat(v, true)
				v, found := s.kv[k]
				if !found {
					s.kv[k] = []byte(bigFloatToStr(v1))
					continue
				}
				v0 := foundOrZeroBigFloat(v, true)

				s.kv[k] = []byte(bigFloatToStr(min(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", s.updatePolicy, s.valueType)
		}
	default:
		return fmt.Errorf("update policy %q not supported", s.updatePolicy) // should have been validated already
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
	bi := &big.Int{}
	_, success := bi.SetString(in, 10)
	if !success {
		panic(fmt.Sprintf("cannot load int %q", in))
	}
	return bi
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
