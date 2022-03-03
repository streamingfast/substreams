package state

import "math/big"

type Reader interface {
	GetFirst(key string) ([]byte, bool)
	GetLast(key string) ([]byte, bool)
	GetAt(ord uint64, key string) ([]byte, bool)
}

type UpdateKeySetter interface {
	Set(ord uint64, key string, value string)
	SetBytes(ord uint64, key string, value []byte)
}

type ConditionalKeySetter interface {
	SetIfNotExists(ord uint64, key string, value string)
	SetBytesIfNotExists(ord uint64, key string, value []byte)
}

type Deleter interface {
	// Deletes a range of keys, lexicographically between `lowKey` and `highKey`
	DeleteRange(lowKey, highKey string)
	// Deletes a range of keys, first considering the _value_ of such keys as a _pointerSeparator_-separated list of keys to _also_ delete.
	DeleteRangePointers(lowKey, highKey, pointerSeparator string)
}

type MaxBigIntSetter interface {
	SetMaxBigInt(ord uint64, key string, value *big.Int)
}
type MaxInt64Setter interface {
	SetMaxInt64(ord uint64, key string, value int64)
}
type MaxFloat64Setter interface {
	SetMaxFloat64(ord uint64, key string, value float64)
}
type MaxBigFloatSetter interface {
	SetMaxBigFloat(ord uint64, key string, value *big.Float)
}

type MinBigIntSetter interface {
	SetMinBigInt(ord uint64, key string, value *big.Int)
}
type MinInt64Setter interface {
	SetMinInt64(ord uint64, key string, value int64)
}
type MinFloat64Setter interface {
	SetMinFloat64(ord uint64, key string, value float64)
}
type MinBigFloatSetter interface {
	SetMinBigFloat(ord uint64, key string, value *big.Float)
}

type SumBigIntSetter interface {
	SumBigInt(ord uint64, key string, value *big.Int)
}
type SumInt64Setter interface {
	SumInt64(ord uint64, key string, value int64)
}
type SumFloat64Setter interface {
	SumFloat64(ord uint64, key string, value float64)
}
type SumBigFloatSetter interface {
	SumBigFloat(ord uint64, key string, value *big.Float)
}

type Mergeable interface {
	Merge(other *Builder) error
}
