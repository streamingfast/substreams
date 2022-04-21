package wasm

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
)

type InputType int

const (
	InputSource InputType = iota
	InputStore
	OutputStore
)

type Input struct {
	Type InputType
	Name string

	// Transient data between calls
	StreamData []byte

	// InputType == InputStore || OutputStore
	Store  *state.Builder
	Deltas bool // whether we want to have the Deltas instead of an access to the store

	// If InputType == OutputStore
	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string
}
