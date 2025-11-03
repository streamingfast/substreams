package wasm

import (
	"fmt"

	"github.com/protocolbuffers/protoscope"
	pbservice "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/service/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
)

const ClockType = "sf.substreams.v1.Clock"

type InputType int

type Argument interface {
	Name() string
}

type ProtoScopeValueArgument interface {
	ProtoScopeValue([]byte) string
}

// implementations

type BaseArgument struct {
	name         string
	initialBlock uint64
}

func (b *BaseArgument) Name() string {
	return b.name
}

type SourceInput struct {
	BaseArgument
}

func NewSourceInput(name string, initialBlock uint64) *SourceInput {
	return &SourceInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
	}
}

func (i *SourceInput) ProtoScopeValue(value []byte) string {
	return "{" + protoscope.Write(value, protoscope.WriterOptions{}) + "}"
}

type MapInput struct {
	BaseArgument
}

func NewMapInput(name string, initialBlock uint64) *MapInput {
	return &MapInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
	}
}

func (i *MapInput) ProtoScopeValue(value []byte) string {
	return fmt.Sprintf("%d", value)
}

type StoreDeltaInput struct {
	BaseArgument
}

func NewStoreDeltaInput(name string, initialBlock uint64) *StoreDeltaInput {
	return &StoreDeltaInput{
		BaseArgument: BaseArgument{
			name:         name,
			initialBlock: initialBlock,
		},
	}
}

func (i *StoreDeltaInput) ProtoScopeValue(value []byte) string {
	return "{" + protoscope.Write(value, protoscope.WriterOptions{}) + "}"
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
	value []byte
	BaseArgument
}

func NewParamsInput(value string) *ParamsInput {
	return &ParamsInput{
		BaseArgument: BaseArgument{
			name:         "params",
			initialBlock: 0,
		},
		value: []byte(value),
	}
}

func (i *ParamsInput) Value() []byte {
	return i.value
}

func (i *ParamsInput) ProtoScopeValue(value []byte) string {
	//todo: need to encode the value
	return fmt.Sprintf("{\"%s\"}", string(value))
}

type FoundationalStoreInput struct {
	BaseArgument
	Clients []pbservice.StoreClient
}

func NewFoundationalStoreInput(name string, clients []pbservice.StoreClient) *FoundationalStoreInput {
	return &FoundationalStoreInput{
		BaseArgument: BaseArgument{name: name},
		Clients:      clients,
	}
}

func (f *FoundationalStoreInput) Name() string {
	return f.name
}

func (f *FoundationalStoreInput) ProtoScopeValue(value []byte) string {
	return fmt.Sprintf("<foundational %s>", string(value))
}
