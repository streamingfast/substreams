package v8

import (
	"context"

	"github.com/streamingfast/substreams/wasm"
	"rogchap.com/v8go"
)

type V8ModuleFactory struct{}

func NewModule(ctx context.Context, wasmCode []byte, wasmCodeType string, registry *wasm.Registry) (wasm.Module, error) {
	// An Isolate is a V8 virtual machine instance that creates an isolated
	// JavaScript runtime environment. Each Substreams module runs in its own
	// isolate to ensure sandboxing and avoid sync errors between runs.
	iso := v8go.NewIsolate()
	return &V8Module{
		code:     wasmCode,
		registry: registry,
		iso:      iso,
	}, nil
}
