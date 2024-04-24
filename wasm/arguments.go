package wasm

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
)

const ClockType = "sf.substreams.v1.Clock"

type InputType int

type Argument interface {
	Name() string
}

type ValueArgument interface {
	Argument
	Value() []byte
	SetValue([]byte)
	Active(blk uint64) bool
}

// implementations

type BaseArgument struct {
	name         string
	initialBlock uint64
}

func (b *BaseArgument) Name() string {
	return b.name
}

func (b *BaseArgument) Active(blk uint64) bool {
	return blk >= b.initialBlock
}

type BaseValueArgument struct {
	value []byte
}

func (b *BaseValueArgument) Value() []byte        { return b.value }
func (b *BaseValueArgument) SetValue(data []byte) { b.value = data }

type SourceInput struct {
	BaseArgument
	BaseValueArgument
}

func NewSourceInput(name string, initialBlock uint64) *SourceInput {
	return &SourceInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
	}
}

type MapInput struct {
	BaseArgument
	BaseValueArgument
}

func NewMapInput(name string, initialBlock uint64) *MapInput {
	return &MapInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
	}
}

type StoreDeltaInput struct {
	BaseArgument
	BaseValueArgument
}

func NewStoreDeltaInput(name string, initialBlock uint64) *StoreDeltaInput {
	return &StoreDeltaInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
	}
}

type StoreReaderInput struct {
	BaseArgument
	Store store.Store
}

func NewStoreReaderInput(name string, store store.Store, initialBlock uint64) *StoreReaderInput {
	return &StoreReaderInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
		Store: store,
	}
}

type StoreWriterOutput struct {
	BaseArgument
	Store        store.Store
	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string
}

func NewStoreWriterOutput(name string, store store.Store, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string) *StoreWriterOutput {
	return &StoreWriterOutput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: 0,
		},
		Store:        store,
		UpdatePolicy: updatePolicy,
		ValueType:    valueType,
	}
}

type ParamsInput struct {
	BaseArgument
	BaseValueArgument
}

func NewParamsInput(value string) *ParamsInput {
	return &ParamsInput{
		BaseArgument: BaseArgument{
			name:         "params",
			initialBlock: 0,
		},
		BaseValueArgument: BaseValueArgument{
			value: []byte(value),
		},
	}
}
