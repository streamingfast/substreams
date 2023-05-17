package wasm

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WASMExtensioner interface {
	WASMExtensions() map[string]map[string]WASMExtension
}

// WASMExtension defines the implementation of a function that will
// be exposed as wasm imports; therefore, exposed to the host language
// like Rust.
//
// For example, this can be an RPC call, taking a structured request
// in `in` and outputting a structured response in `out`, both
// serialized as protobuf messages.
//
// Such a function needs to be registered through RegisterRuntime.
type WASMExtension func(ctx context.Context, traceID string, clock *pbsubstreams.Clock, in []byte) (out []byte, err error)

// WASM VM specific implementation to create a new Module, which is an abstraction
// around a runtime and pre-compiled WASM modules.
type ModuleFactory interface {
	NewModule(ctx context.Context, code []byte, registry *Registry) (module Module, err error)
}

type ModuleFactoryFunc func(ctx context.Context, wasmCode []byte, registry *Registry) (module Module, err error)

func (f ModuleFactoryFunc) NewModule(ctx context.Context, wasmCode []byte, registry *Registry) (module Module, err error) {
	return f(ctx, wasmCode, registry)
}

// A Module is a cached or pre-compiled version able to generate new isolated
// instances, and execute calls on them.
type Module interface {
	ExecuteNewCall(ctx context.Context, call *Call, cachedInstance Instance, arguments []Argument) (instance Instance, err error)
}

type Instance interface {
	Close(ctx context.Context) error
}

var runtimes = map[string]ModuleFactory{}

func RegisterModuleFactory(name string, factory ModuleFactory) {
	runtimes[name] = factory
}
