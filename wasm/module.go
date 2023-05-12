package wasm

import (
	"context"
	"fmt"

	tracing "github.com/streamingfast/sf-tracing"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// A Module represents a wazero.Runtime that clears and is destroyed upon completion of a request.
// It has the pre-compiled `env` host module, as well as pre-compiled WASM code provided by the user
type Module struct {
	registry *Registry

	wazRuntime      wazero.Runtime
	wazModuleConfig wazero.ModuleConfig
	hostModules     []wazero.CompiledModule
	userModule      wazero.CompiledModule
}

func (r *Registry) NewModule(wasmCode []byte) (*Module, error) {
	// What's the effect of `ctx` here? Will it kill all the WASM if it cancels?
	// TODO: try with: wazero.NewRuntimeConfigCompiler()
	// TODO: try config := wazero.NewRuntimeConfig().WithCompilationCache(cache)
	ctx := context.Background()
	runtimeConfig := wazero.NewRuntimeConfigCompiler()
	// TODO: can we use some caching in the RuntimeConfig so perhaps we reuse
	// things across runtimes creations?
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	hostModules, err := addExtensionFunctions(ctx, runtime, r)
	if err != nil {
		return nil, err
	}
	envModule, err := addHostFunctions(ctx, runtime, "env", envFuncs)
	if err != nil {
		return nil, err
	}
	stateModule, err := addHostFunctions(ctx, runtime, "state", stateFuncs)
	if err != nil {
		return nil, err
	}
	loggerModule, err := addHostFunctions(ctx, runtime, "logger", loggerFuncs)
	if err != nil {
		return nil, err
	}
	hostModules = append(hostModules, envModule, stateModule, loggerModule)

	// TODO: where to `Close()` the `runtime` here?
	// One runtime per request?
	mod, err := runtime.CompileModule(ctx, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("creating new module: %w", err)
	}

	funcs := mod.ExportedFunctions()
	if funcs["alloc"] == nil {
		return nil, fmt.Errorf("missing required functions: alloc")
	}
	if funcs["dealloc"] == nil {
		return nil, fmt.Errorf("missing required functions: dealloc")
	}

	return &Module{
		registry:        r,
		wazModuleConfig: wazero.NewModuleConfig(),
		wazRuntime:      runtime,
		userModule:      mod,
		hostModules:     hostModules,
	}, nil
}

func (m *Module) instantiateModule(ctx context.Context) (api.Module, error) {
	for _, hostMod := range m.hostModules {
		if m.wazRuntime.Module(hostMod.Name()) != nil {
			continue
		}
		_, err := m.wazRuntime.InstantiateModule(ctx, hostMod, m.wazModuleConfig.WithName(hostMod.Name()))
		if err != nil {
			return nil, fmt.Errorf("instantiating host module %q: %w", hostMod.Name(), err)
		}
	}
	mod, err := m.wazRuntime.InstantiateModule(ctx, m.userModule, m.wazModuleConfig.WithName(""))
	return mod, err
}

type parm = api.ValueType

var i32 = api.ValueTypeI32
var i64 = api.ValueTypeI64
var f64 = api.ValueTypeF64

func addExtensionFunctions(ctx context.Context, runtime wazero.Runtime, registry *Registry) (out []wazero.CompiledModule, err error) {
	for namespace, imports := range registry.extensions {
		builder := runtime.NewHostModuleBuilder(namespace)
		for importName, f := range imports {
			builder.NewFunctionBuilder().
				WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
					ptr, length, outputPtr := uint32(stack[0]), uint32(stack[1]), uint32(stack[2])
					data := readBytes(mod, ptr, length)
					call := fromContext(ctx)
					traceID := tracing.GetTraceID(ctx).String()

					out, err := f(ctx, traceID, call.clock, data)
					if err != nil {
						panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, importName, err))
					}

					if ctx.Err() == context.Canceled {
						// Sometimes long-running extensions will come back to a canceled context.
						// so avoid writing to memory then
						return
					}

					if err := call.writeOutputToHeap(ctx, mod, outputPtr, out, importName); err != nil {
						panic(fmt.Errorf("write output to heap %w", err))
					}
				}), []parm{i32, i32, i32}, []parm{}).
				Export(importName)
		}
		mod, err := builder.Compile(ctx)
		if err != nil {
			return nil, fmt.Errorf("compiling wasm extension %q: %w", namespace, err)
		}
		out = append(out, mod)
	}
	return
}

//func addExtensionFunction

func addHostFunctions(ctx context.Context, runtime wazero.Runtime, moduleName string, funcs []funcs) (wazero.CompiledModule, error) {
	build := runtime.NewHostModuleBuilder(moduleName)
	for _, f := range funcs {
		build.NewFunctionBuilder().
			WithGoModuleFunction(f.f, f.input, f.output).
			WithName(f.name).
			Export(f.name)
	}
	return build.Compile(ctx)
}

type funcs struct {
	name  string
	input []parm
	//inputNames  []string
	output []parm
	f      api.GoModuleFunction
}

func readBytesFromStack(mod api.Module, stack []uint64) []byte {
	ptr, length := uint32(stack[0]), uint32(stack[1])
	return readBytes(mod, ptr, length)
}
func readStringFromStack(mod api.Module, stack []uint64) string {
	ptr, length := uint32(stack[0]), uint32(stack[1])
	return readString(mod, ptr, length)
}

func readString(mod api.Module, ptr, len uint32) string {
	bytes, ok := mod.Memory().Read(ptr, len)
	if !ok {
		panic(fmt.Sprintf("could not read string, ptr=%d, len=%d", ptr, len))
	}
	return string(bytes)
}

func readBytes(mod api.Module, ptr, length uint32) []byte {
	bytes, ok := mod.Memory().Read(ptr, length)
	if !ok {
		panic(fmt.Sprintf("could not read string, ptr=%d, len=%d", ptr, length))
	}
	return bytes
}
