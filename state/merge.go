package state

import (
	"fmt"
	"math/big"
	"strconv"
)

const (
	UpdatePolicyReplace = "replace"
	UpdatePolicyIgnore  = "ignore"
	UpdatePolicySum     = "sum"
	UpdatePolicyMax     = "max"
	UpdatePolicyMin     = "min"
)

const (
	OutputValueTypeInt64    = "int64"
	OutputValueTypeBigFloat = "bigFloat"
	OutputValueTypeString   = "string"
)

func (b *Builder) Merge(previous *Builder) error {
	latest := b

	if latest.updatePolicy != previous.updatePolicy {
		return fmt.Errorf("incompatible update policies: policy %q cannot merge policy %q", latest.updatePolicy, previous.updatePolicy)
	}

	if latest.valueType != previous.valueType { //TODO: will we one day want to be able to merge numeric types int and float?
		return fmt.Errorf("incompatible value types: cannot merge %q and %q", latest.valueType, previous.valueType)
	}

	switch latest.updatePolicy {
	case UpdatePolicyReplace:
		for k, v := range previous.KV {
			if _, found := latest.KV[k]; !found {
				latest.KV[k] = v
			}
		}
	case UpdatePolicyIgnore:
		for k, v := range previous.KV {
			if _, found := latest.KV[k]; found {
				latest.KV[k] = v
			}
		}
	case UpdatePolicySum:
		// check valueType to do the right thing
		switch latest.valueType {
		case OutputValueTypeInt64:
			sum := func(a, b uint64) uint64 {
				return a + b
			}
			for k, v := range previous.KV {
				v0b, fv0 := latest.KV[k]
				v0 := foundOrZeroUint64(v0b, fv0)
				v1 := foundOrZeroUint64(v, true)
				latest.KV[k] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			sum := func(a, b *big.Float) *big.Float {
				return bf().Add(a, b).SetPrec(100)
			}
			for k, v := range previous.KV {
				v0b, fv0 := latest.KV[k]
				v0 := foundOrZeroFloat(v0b, fv0)
				v1 := foundOrZeroFloat(v, true)
				latest.KV[k] = []byte(floatToStr(sum(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", latest.updatePolicy, latest.valueType)
		}
	case UpdatePolicyMax:
		switch latest.valueType {
		case OutputValueTypeInt64:
			max := func(a, b uint64) uint64 {
				if a >= b {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroUint64(v, true)
				v, found := latest.KV[k]
				if !found {
					latest.KV[k] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroUint64(v, true)

				latest.KV[k] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			max := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroFloat(v, true)
				v, found := latest.KV[k]
				if !found {
					latest.KV[k] = []byte(floatToStr(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				latest.KV[k] = []byte(floatToStr(max(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", latest.updatePolicy, latest.valueType)
		}
	case UpdatePolicyMin:
		switch latest.valueType {
		case OutputValueTypeInt64:
			min := func(a, b uint64) uint64 {
				if a <= b {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroUint64(v, true)
				v, found := latest.KV[k]
				if !found {
					latest.KV[k] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroUint64(v, true)

				latest.KV[k] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case OutputValueTypeBigFloat:
			min := func(a, b *big.Float) *big.Float {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for k, v := range previous.KV {
				v1 := foundOrZeroFloat(v, true)
				v, found := latest.KV[k]
				if !found {
					latest.KV[k] = []byte(floatToStr(v1))
					continue
				}
				v0 := foundOrZeroFloat(v, true)

				latest.KV[k] = []byte(floatToStr(min(v0, v1)))
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %s", latest.updatePolicy, latest.valueType)
		}
	default:
		return fmt.Errorf("update policy %q not supported", latest.updatePolicy) // should have been validated already
	}

	latest.bundler = nil //todo(colin): is this required?
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
