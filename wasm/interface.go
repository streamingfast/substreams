package wasm

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WASMExtensioner interface {
	Params() map[string]string // tier1 gives me the params directly, tier2 would return nil
	WASMExtensions(map[string]string) (map[string]map[string]WASMExtension, error)
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
type WASMExtension func(ctx context.Context, requestID string, clock *pbsubstreams.Clock, in []byte) (out []byte, err error)

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
// instances, and execute calls on them. It lives for the duration of a stream.
type Module interface {
	// NewInstance can be used to create up-front a new instance, which will be
	// cached and reused for the duration execution of ExecuteNewCall.
	NewInstance(ctx context.Context) (instance Instance, err error)

	// ExecuteNewCall is called once per module execution for each block.
	// If caching is enabled, the returned Instance will be saved and passed in
	// as the `cachedInstance` argument upon the next call. In which case, the runtime
	// would benefit from using it back. It is the runtime's responsibility to determine
	// whether this caching entails risks to determinism (leaking of global state for instance).
	ExecuteNewCall(ctx context.Context, call *Call, cachedInstance Instance, arguments []Argument) (instance Instance, err error)

	// Close gets called when the module can be unloaded at the end of a user's request.
	Close(ctx context.Context) error
}

// An Instance lives for the duration of an execution (with instance caching disabled)
// // or a series of execution (when instance caching is enabled).
type Instance interface {
	// Cleanup is called between each calls, for lightweight clean-up (remove allocation)
	// in case we're not using a fully deterministic execution strategy.
	Cleanup(ctx context.Context) error

	// Close is called once we know we won't be reusing this instance, and it can be
	// freed from memory.  When using cached instances, this won't be called between
	// each execution, but only at the end of a user's request.
	Close(ctx context.Context) error
}

var runtimes = map[string]ModuleFactory{}

func RegisterModuleFactory(name string, factory ModuleFactory) {
	runtimes[name] = factory
}
