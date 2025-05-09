package v8

import (
	"context"

	"github.com/streamingfast/substreams/wasm"
	"rogchap.com/v8go"
)

type V8ModuleFactory struct{}

func NewModule(ctx context.Context, wasmCode []byte, wasmCodeType string, registry *wasm.Registry) (wasm.Module, error) {
	iso := v8go.NewIsolate()
	return &V8Module{
		code:     wasmCode,
		registry: registry,
		iso:      iso,
	}, nil
}
