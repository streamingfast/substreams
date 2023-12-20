package wasm

import (
	"fmt"

	"github.com/protocolbuffers/protoscope"
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
}

type ProtoScopeValueArgument interface {
	ProtoScopeValue() []byte
}

// implementations

type BaseArgument struct {
	name string
}

func (b *BaseArgument) Name() string {
	return b.name
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

func NewSourceInput(name string) *SourceInput {
	return &SourceInput{
		BaseArgument: BaseArgument{
			name: name,
		},
	}
}

func (i *SourceInput) ProtoScopeValue() string {
	return "{" + protoscope.Write(i.value, protoscope.WriterOptions{}) + "}"
}

type MapInput struct {
	BaseArgument
	BaseValueArgument
}

func NewMapInput(name string) *MapInput {
	return &MapInput{
		BaseArgument: BaseArgument{
			name: name,
		},
	}
}

func (i *MapInput) ProtoScopeValue() string {
	return fmt.Sprintf("%d", i.value)
}

type StoreDeltaInput struct {
	BaseArgument
	BaseValueArgument
}

func NewStoreDeltaInput(name string) *StoreDeltaInput {
	return &StoreDeltaInput{
		BaseArgument: BaseArgument{
			name: name,
		},
	}
}

func (i *StoreDeltaInput) ProtoScopeValue() string {
	return "{" + protoscope.Write(i.value, protoscope.WriterOptions{}) + "}"
}

type StoreReaderInput struct {
	BaseArgument
	Store store.Store
}

func NewStoreReaderInput(name string, store store.Store) *StoreReaderInput {
	return &StoreReaderInput{
		BaseArgument: BaseArgument{
			name: name,
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
			name: name,
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
			name: "params",
		},
		BaseValueArgument: BaseValueArgument{
			value: []byte(value),
		},
	}
}
func (i *ParamsInput) ProtoScopeValue() string {
	return fmt.Sprintf("{%s}", string(i.value))
}
