package store

import (
	"context"
	"fmt"
	"math/big"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Store interface {
	Name() string
	InitialBlock() uint64

	Loadable
	Savable
	Iterable
	DeltaAccessor
	Resettable
	Mergeable
	Named
	// todoo: add fmt.Stringer ??

	// intrinsics
	Reader

	UpdateKeySetter
	ConditionalKeySetter
	Appender
	Deleter

	MaxBigIntSetter
	MaxInt64Setter
	MaxFloat64Setter
	MaxBigDecimalSetter

	MinBigIntSetter
	MinInt64Setter
	MinFloat64Setter
	MinBigDecimalSetter

	SumBigIntSetter
	SumInt64Setter
	SumFloat64Setter
	SumBigDecimalSetter
}

type PartialStore interface {
	Roll(lastBlock uint64)
}

type Loadable interface {
	Load(ctx context.Context, atBlock uint64) error
}

type Savable interface {
	Save(endBoundaryBlock uint64) (*block.Range, *fileWriter, error)
}

type Resettable interface {
	Reset()
}

type Named interface {
	Name() string
}

type Iterable interface {
	Length() uint64
	Iter(func(key string, value []byte) error) error
}

type DeltaAccessor interface {
	SetDeltas([]*pbsubstreams.StoreDelta)
	GetDeltas() []*pbsubstreams.StoreDelta
	ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta)
	ApplyDelta(delta *pbsubstreams.StoreDelta)
}

type Reader interface {
	fmt.Stringer

	Named

	GetFirst(key string) ([]byte, bool)
	GetLast(key string) ([]byte, bool)
	GetAt(ord uint64, key string) ([]byte, bool)

	HasFirst(key string) bool
	HasLast(key string) bool
	HasAt(ord uint64, key string) bool
}

type Mergeable interface {
	ValueType() string
	UpdatePolicy() pbsubstreams.Module_KindStore_UpdatePolicy
}

type UpdateKeySetter interface {
	Set(ord uint64, key string, value string)
	SetBytes(ord uint64, key string, value []byte)
}

type ConditionalKeySetter interface {
	SetIfNotExists(ord uint64, key string, value string)
	SetBytesIfNotExists(ord uint64, key string, value []byte)
}

type Appender interface {
	Append(ord uint64, key string, value []byte) error
}

type Deleter interface {
	DeletePrefix(ord uint64, prefix string)
	//// Deletes a range of keys, lexicographically between `lowKey` and `highKey`
	//DeleteRange(lowKey, highKey string)
	//// Deletes a range of keys, first considering the _value_ of such keys as a _pointerSeparator_-separated list of keys to _also_ delete.
	//DeleteRangePointers(lowKey, highKey, pointerSeparator string)
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
type MaxBigDecimalSetter interface {
	SetMaxBigDecimal(ord uint64, key string, value *big.Float)
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
type MinBigDecimalSetter interface {
	SetMinBigDecimal(ord uint64, key string, value *big.Float)
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
type SumBigDecimalSetter interface {
	SumBigDecimal(ord uint64, key string, value *big.Float)
}
