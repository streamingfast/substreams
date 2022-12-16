package store

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/manifest"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) setKV(k string, v []byte) {
	if prev, ok := b.kv[k]; ok {
		b.totalSizeBytes -= uint64(len(prev))
	} else {
		b.totalSizeBytes += uint64(len(k))
	}
	b.totalSizeBytes += uint64(len(v))
	b.kv[k] = v
}

func (b *baseStore) setNewKV(k string, v []byte) {
	b.totalSizeBytes += uint64(len(k) + len(v))
	b.kv[k] = v
}

// Merge nextStore _into_ `s`, where nextStore is for the next contiguous segment's store output.
func (b *baseStore) Merge(kvPartialStore *PartialKV) error {
	b.logger.Debug("merging store", zap.Int("current_key_count", len(b.kv)), zap.Uint64("mod_init_block", b.moduleInitialBlock), zap.Int("partial_key_count", len(kvPartialStore.kv)), zap.Uint64("partial_start_block", kvPartialStore.initialBlock))

	if kvPartialStore.updatePolicy != b.updatePolicy {
		return fmt.Errorf("incompatible update policies: policy %q cannot merge policy %q", b.updatePolicy, kvPartialStore.updatePolicy)
	}

	if kvPartialStore.valueType != b.valueType {
		return fmt.Errorf("incompatible value types: cannot merge %q and %q", b.valueType, kvPartialStore.valueType)
	}

	for _, prefix := range kvPartialStore.DeletedPrefixes {
		b.DeletePrefix(kvPartialStore.lastOrdinal, prefix)
	}

	intoValueTypeLower := strings.ToLower(b.valueType)

	switch b.updatePolicy {
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET:
		for k, v := range kvPartialStore.kv {
			b.setKV(k, v)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS:
		for k, v := range kvPartialStore.kv {
			if _, found := b.kv[k]; !found {
				b.setNewKV(k, v)
			}
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND:
		for k, v := range kvPartialStore.kv {
			if prevVal, found := b.kv[k]; found {
				newLen := len(prevVal) + len(v)
				if b.appendLimit > 0 && uint64(newLen) >= b.appendLimit {
					return fmt.Errorf("append would exceed limit of %d bytes", b.appendLimit)
				}

				nextVal := make([]byte, len(prevVal)+len(v))
				copy(nextVal[0:], prevVal)
				copy(nextVal[len(prevVal):], v)
				b.setKV(k, nextVal)
			} else {
				b.setNewKV(k, v)
			}
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD:
		// check valueType to do the right thing
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			sum := func(a, b int64) int64 {
				return a + b
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := b.kv[k]
				v0 := foundOrZeroInt64(v0b, fv0)
				v1 := foundOrZeroInt64(v, true)
				b.setKV(k, []byte(fmt.Sprintf("%d", sum(v0, v1))))
			}
		case manifest.OutputValueTypeFloat64:
			sum := func(a, b float64) float64 {
				return a + b
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := b.kv[k]
				v0 := foundOrZeroFloat(v0b, fv0)
				v1 := foundOrZeroFloat(v, true)
				b.setKV(k, floatToBytes(sum(v0, v1)))
			}
		case manifest.OutputValueTypeBigInt:
			sum := func(a, b *big.Int) *big.Int {
				return new(big.Int).Add(a, b)
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := b.kv[k]
				v0 := foundOrZeroBigInt(v0b, fv0)
				v1 := foundOrZeroBigInt(v, true)
				b.setKV(k, []byte(fmt.Sprintf("%d", sum(v0, v1))))
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			sum := func(a, b *big.Float) *big.Float {
				return new(big.Float).SetPrec(100).Add(a, b).SetPrec(100)
			}
			for k, v := range kvPartialStore.kv {
				v0b, fv0 := b.kv[k]
				v0 := foundOrZeroBigFloat(v0b, fv0)
				v1 := foundOrZeroBigFloat(v, true)
				b.setKV(k, bigFloatToBytes(sum(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %q", b.updatePolicy, b.valueType)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX:
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			max := func(a, b int64) int64 {
				if a >= b {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroInt64(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, []byte(fmt.Sprintf("%d", v1)))
					continue
				}
				v0 := foundOrZeroInt64(v, true)

				b.setKV(k, []byte(fmt.Sprintf("%d", max(v0, v1))))
			}
		case manifest.OutputValueTypeFloat64:
			max := func(a, b float64) float64 {
				if a < b {
					return b
				}
				return a
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroFloat(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, floatToBytes(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				b.setKV(k, floatToBytes(max(v0, v1)))
			}
		case manifest.OutputValueTypeBigInt:
			max := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigInt(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, []byte(v1.String()))
					continue
				}
				v0 := foundOrZeroBigInt(v, true)

				b.setKV(k, []byte(fmt.Sprintf("%d", max(v0, v1))))
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			max := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigFloat(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, bigFloatToBytes(v1))
					continue
				}
				v0 := foundOrZeroBigFloat(v, true)

				b.setKV(k, bigFloatToBytes(max(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %q", kvPartialStore.updatePolicy, kvPartialStore.valueType)
		}
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN:
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			min := func(a, b int64) int64 {
				if a <= b {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroInt64(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, []byte(fmt.Sprintf("%d", v1)))
					continue
				}
				v0 := foundOrZeroInt64(v, true)

				b.setKV(k, []byte(fmt.Sprintf("%d", min(v0, v1))))
			}
		case manifest.OutputValueTypeFloat64:
			min := func(a, b float64) float64 {
				if a < b {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroFloat(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, floatToBytes(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				b.setKV(k, floatToBytes(min(v0, v1)))
			}
		case manifest.OutputValueTypeBigInt:
			min := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigInt(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, []byte(v1.String()))
					continue
				}
				v0 := foundOrZeroBigInt(v, true)

				b.setKV(k, []byte(fmt.Sprintf("%d", min(v0, v1))))
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			min := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range kvPartialStore.kv {
				v1 := foundOrZeroBigFloat(v, true)
				v, found := b.kv[k]
				if !found {
					b.setNewKV(k, bigFloatToBytes(v1))
					continue
				}
				v0 := foundOrZeroBigFloat(v, true)
				b.setKV(k, bigFloatToBytes(min(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %q", b.updatePolicy, b.valueType)
		}
	default:
		return fmt.Errorf("update policy %q not supported", b.updatePolicy) // should have been validated already
	}

	return nil
}

func foundOrZeroInt64(in []byte, found bool) int64 {
	if !found {
		return 0
	}
	val, err := strconv.ParseInt(string(in), 10, 64)
	if err != nil {
		return 0
	}
	return int64(val)
}

func foundOrZeroBigFloat(in []byte, found bool) *big.Float {
	if !found {
		return new(big.Float).SetPrec(100)
	}
	return bytesToBigFloat(in)
}

func foundOrZeroBigInt(in []byte, found bool) *big.Int {
	if !found {
		return new(big.Int)
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

func bigFloatToStr(f *big.Float) string {
	return f.Text('g', -1)
}

func bigFloatToBytes(f *big.Float) []byte {
	return []byte(bigFloatToStr(f))
}
