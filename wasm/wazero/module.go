package wazero

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
)

// A Module represents a wazero.Runtime that clears and is destroyed upon completion of a request.
// It has the pre-compiled `env` host module, as well as pre-compiled WASM code provided by the user
type Module struct {
	sync.Mutex
	wazRuntime      wazero.Runtime
	wazModuleConfig wazero.ModuleConfig
	hostModules     []wazero.CompiledModule
	userModule      wazero.CompiledModule
}

func init() {
	wasm.RegisterModuleFactory("wazero", wasm.ModuleFactoryFunc(newModule))
}

func newModule(ctx context.Context, wasmCode []byte, registry *wasm.Registry) (wasm.Module, error) {
	// What's the effect of `ctx` here? Will it kill all the WASM if it cancels?
	// TODO: try with: wazero.NewRuntimeConfigCompiler()
	// TODO: try config := wazero.NewRuntimeConfig().WithCompilationCache(cache)
	runtimeConfig := wazero.NewRuntimeConfigCompiler()
	// TODO: can we use some caching in the RuntimeConfig so perhaps we reuse
	// things across runtimes creations?
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	hostModules, err := addExtensionFunctions(ctx, runtime, registry)
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
		wazModuleConfig: wazero.NewModuleConfig(),
		wazRuntime:      runtime,
		userModule:      mod,
		hostModules:     hostModules,
	}, nil
}

func (m *Module) Close(ctx context.Context) error {
	closeFuncs := []func(context.Context) error{
		m.wazRuntime.Close,
		m.userModule.Close,
	}
	for _, hostMod := range m.hostModules {
		closeFuncs = append(closeFuncs, hostMod.Close)
	}
	for _, f := range closeFuncs {
		if err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) NewInstance(ctx context.Context) (out wasm.Instance, err error) {
	mod, err := m.instantiateModule(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not instantiate wasm module: %w", err)
	}

	return &instance{Module: mod}, nil
}

func (m *Module) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument) (out wasm.Instance, err error) {
	var mod api.Module
	if cachedInstance != nil {
		mod = cachedInstance.(api.Module)
	} else {
		mod, err = m.instantiateModule(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not instantiate wasm module: %w", err)
		}
	}
	inst := &instance{Module: mod}

	f := mod.ExportedFunction(call.Entrypoint)
	if f == nil {
		return inst, fmt.Errorf("could not find entrypoint function %q ", call.Entrypoint)
	}

	var args []uint64
	var inputStoreCount int
	for _, input := range arguments {
		switch v := input.(type) {
		case *wasm.StoreWriterOutput:
		case *wasm.StoreReaderInput:
			inputStoreCount++
			args = append(args, uint64(inputStoreCount-1))
		case wasm.ValueArgument:
			cnt := v.Value()
			ptr, err := writeToHeap(ctx, inst, true, cnt)
			if err != nil {
				return nil, fmt.Errorf("writing %s to heap: %w", input.Name(), err)
			}
			length := uint64(len(cnt))
			args = append(args, uint64(ptr), length)
		default:
			panic("unknown wasm argument type")
		}
	}

	_, err = f.Call(wasm.WithContext(withInstanceContext(ctx, inst), call), args...)
	if err != nil {
		return inst, fmt.Errorf("call: %w", err)
	}

	return inst, nil
}

func (m *Module) instantiateModule(ctx context.Context) (api.Module, error) {
	m.Lock()
	defer m.Unlock()

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

func addExtensionFunctions(ctx context.Context, runtime wazero.Runtime, registry *wasm.Registry) (out []wazero.CompiledModule, err error) {
	for namespace, imports := range registry.Extensions {
		builder := runtime.NewHostModuleBuilder(namespace)
		for importName, f := range imports {
			builder.NewFunctionBuilder().
				WithGoFunction(api.GoFunc(func(ctx context.Context, stack []uint64) {
					inst := instanceFromContext(ctx)
					ptr, length, outputPtr := uint32(stack[0]), uint32(stack[1]), uint32(stack[2])
					data := readBytes(inst, ptr, length)
					call := wasm.FromContext(ctx)

					metricID := reqctx.ReqStats(ctx).RecordModuleWasmExternalCallBegin(call.ModuleName, fmt.Sprintf("%s:%s", namespace, importName))

					out, err := f(ctx, reqctx.Details(ctx).UniqueIDString(), call.Clock, data)
					if err != nil {
						panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, importName, err))
					}

					reqctx.ReqStats(ctx).RecordModuleWasmExternalCallEnd(call.ModuleName, fmt.Sprintf("%s:%s", namespace, importName), metricID)

					if ctx.Err() == context.Canceled {
						// Sometimes long-running extensions will come back to a canceled context.
						// so avoid writing to memory then
						return
					}

					if err := writeOutputToHeap(ctx, inst, outputPtr, out); err != nil {
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
