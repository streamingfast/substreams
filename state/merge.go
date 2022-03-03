package state

import (
	"fmt"
	"math/big"
	"strconv"
)

const (
	MergeStrategyLastKey   = "LAST_KEY"
	MergeStrategySumInts   = "SUM_INTS"
	MergeStrategySumFloats = "SUM_FLOATS"
	MergeStrategyMinInt    = "MIN_INT"
	MergeStrategyMinFloat  = "MIN_FLOAT"
)

func (b *Builder) Merge(next *Builder) error {
	if b.updatePolicy != next.updatePolicy {
		return fmt.Errorf("incompatible update policies: policy %s cannot merge policy %s", b.updatePolicy, next.updatePolicy)
	}

	switch b.updatePolicy {
	case "replace":
		// Last key wins merge strategy
		first := next
		last := b
		for k, v := range first.KV {
			if _, found := last.KV[k]; !found {
				last.KV[k] = v
			}
		}
	case "ignore":
		// First key wins merge strategy
		first := next
		last := b
		for k, v := range first.KV {
			if _, found := last.KV[k]; found {
				last.KV[k] = v
			}
		}
	case "sum":
		// check valueType to do the right thing
	case "max":
		// check valueType to do the right thing
	case "min":
		// check valueType to do the right thing

	// case MergeStrategySumInts:
	// 	first := next
	// 	last := b

	// 	for k, v := range first.KV {
	// 		latestVal, found := last.KV[k]
	// 		if !found {
	// 			last.KV[k] = v
	// 		} else {
	// 			// decode `v` as big.Int, decode `latestVal` as big.Int
	// 			// last.KV[k] = bi().Add(vbigint, latestbigint).String()
	// 		}
	// 	}
	// case MergeStrategySumFloats:
	// 	for k, v := range next.KV {
	// 		v0 := foundOrZeroFloat(b.GetLast(k))
	// 		v1 := foundOrZeroFloat(v, true)
	// 		sum := bf().Add(v0, v1).SetPrec(100)
	// 		b.Set(next.lastOrdinal, k, floatToStr(sum))
	// 	}
	// case MergeStrategyMinInt:
	// 	minInt := func(a, b uint64) uint64 {
	// 		if a < b {
	// 			return a
	// 		}
	// 		return b
	// 	}
	// 	for k, v := range next.KV {
	// 		v1 := foundOrZeroUint64(v, true)

	// 		_, found := b.GetLast(k)
	// 		if !found {
	// 			b.Set(next.lastOrdinal, k, fmt.Sprintf("%d", v1))
	// 		}
	// 		v0 := foundOrZeroUint64(b.GetLast(k))
	// 		b.Set(next.lastOrdinal, k, fmt.Sprintf("%d", minInt(v0, v1)))
	// 	}
	// case MergeStrategyMinFloat:
	// 	minFloat := func(a, b *big.Float) *big.Float {
	// 		if a.Cmp(b) < 1 {
	// 			return a
	// 		}
	// 		return b
	// 	}
	// 	for k, v := range next.KV {
	// 		v1 := foundOrZeroFloat(v, true)

	// 		_, found := b.GetLast(k)
	// 		if !found {
	// 			b.Set(next.lastOrdinal, k, floatToStr(v1))
	// 		}

	// 		v0 := foundOrZeroFloat(b.GetLast(k))

	// 		m := minFloat(v0, v1).SetPrec(100)
	// 		b.Set(next.lastOrdinal, k, floatToStr(m))
	// 	}
	default:
		return fmt.Errorf("unsupported update policy %q", b.updatePolicy) // should have been validated already
	}

	b.bundler = nil

	return nil
}

//TODO(colin): all funcs below are copied from other parts of this repo.  de-duplicate this!

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

func foundOrZeroFloat(in []byte, found bool) *big.Float {
	if !found {
		return bf()
	}
	return bytesToFloat(in)
}

func strToFloat(in string) *big.Float {
	newFloat, _, err := big.ParseFloat(in, 10, 100, big.ToNearestEven)
	if err != nil {
		panic(fmt.Sprintf("cannot load float %q: %s", in, err))
	}
	return newFloat.SetPrec(100)
}

func bytesToFloat(in []byte) *big.Float {
	return strToFloat(string(in))
}

func floatToStr(f *big.Float) string {
	return f.Text('g', -1)
}

func floatToBytes(f *big.Float) []byte {
	return []byte(floatToStr(f))
}

var bf = func() *big.Float { return new(big.Float).SetPrec(100) }
