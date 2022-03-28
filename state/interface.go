package state

import "math/big"

type Reader interface {
	GetFirst(key string) ([]byte, bool, error)
	GetLast(key string) ([]byte, bool)
	GetAt(ord uint64, key string) ([]byte, bool, error)
}

type UpdateKeySetter interface {
	Set(ord uint64, key string, value string) error
	SetBytes(ord uint64, key string, value []byte) error
}

type ConditionalKeySetter interface {
	SetIfNotExists(ord uint64, key string, value string) error
	SetBytesIfNotExists(ord uint64, key string, value []byte) error
}

type Deleter interface {
	DeletePrefix(ord uint64, prefix string) error
	//// Deletes a range of keys, lexicographically between `lowKey` and `highKey`
	//DeleteRange(lowKey, highKey string)
	//// Deletes a range of keys, first considering the _value_ of such keys as a _pointerSeparator_-separated list of keys to _also_ delete.
	//DeleteRangePointers(lowKey, highKey, pointerSeparator string)
}

type MaxBigIntSetter interface {
	SetMaxBigInt(ord uint64, key string, value *big.Int) error
}
type MaxInt64Setter interface {
	SetMaxInt64(ord uint64, key string, value int64) error
}
type MaxFloat64Setter interface {
	SetMaxFloat64(ord uint64, key string, value float64) error
}
type MaxBigFloatSetter interface {
	SetMaxBigFloat(ord uint64, key string, value *big.Float) error
}

type MinBigIntSetter interface {
	SetMinBigInt(ord uint64, key string, value *big.Int) error
}
type MinInt64Setter interface {
	SetMinInt64(ord uint64, key string, value int64) error
}
type MinFloat64Setter interface {
	SetMinFloat64(ord uint64, key string, value float64) error
}
type MinBigFloatSetter interface {
	SetMinBigFloat(ord uint64, key string, value *big.Float) error
}

type SumBigIntSetter interface {
	SumBigInt(ord uint64, key string, value *big.Int) error
}
type SumInt64Setter interface {
	SumInt64(ord uint64, key string, value int64) error
}
type SumFloat64Setter interface {
	SumFloat64(ord uint64, key string, value float64) error
}
type SumBigFloatSetter interface {
	SumBigFloat(ord uint64, key string, value *big.Float) error
}

type Mergeable interface {
	Merge(other *Builder) error
}

//compile-time check that Builder implements all interfaces
var _ interface {
	Reader

	UpdateKeySetter

	MaxBigIntSetter
	MaxInt64Setter
	MaxFloat64Setter
	MaxBigFloatSetter

	MinBigIntSetter
	MinInt64Setter
	MinFloat64Setter
	MinBigFloatSetter

	SumBigIntSetter
	SumInt64Setter
	SumFloat64Setter
	SumBigFloatSetter

	Mergeable
} = (*Builder)(nil)
