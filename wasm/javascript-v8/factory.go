package v8

import (
	"context"

	"github.com/streamingfast/substreams/wasm"
)

type V8ModuleFactory struct{}

func NewModule(ctx context.Context, wasmCode []byte, wasmCodeType string, registry *wasm.Registry) (wasm.Module, error) {
	mod := &V8Module{
		code:     wasmCode,
		registry: registry,
	}
	return mod, nil
}
