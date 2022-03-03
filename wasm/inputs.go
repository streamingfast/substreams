package wasm

import "github.com/streamingfast/substreams/state"

type InputType int

const (
	InputStream InputType = iota
	InputStore
	OutputStore
)

type Input struct {
	Type InputType
	Name string

	// Transient data between calls
	StreamData []byte

	// InputType == InputStore || OutputStore
	Store *state.Builder

	// If InputType == OutputStore
	UpdatePolicy string
	ValueType    string
	ProtoType    string
}
