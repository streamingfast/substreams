package wazero

import (
	"context"
	"fmt"

	tracing "github.com/streamingfast/sf-tracing"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/streamingfast/substreams/wasm"
)

// A Module represents a wazero.Runtime that clears and is destroyed upon completion of a request.
// It has the pre-compiled `env` host module, as well as pre-compiled WASM code provided by the user
type Module struct {
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

func (m *Module) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument) (returnInstance wasm.Instance, err error) {
	//t0 := time.Now()

	var mod api.Module
	if cachedInstance != nil {
		mod = cachedInstance.(api.Module)
	} else {
		//fmt.Println("Instantiate")
		mod, err = m.instantiateModule(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not instantiate wasm module for %q: %w", call.ModuleName, err)
		}
		// Closed by the caller.
		//defer mod.Close(ctx) // Otherwise, deferred to the BaseExecutor.Close() when cached.
	}
	//fmt.Println("Timing 1", time.Since(t0))
	//t0 = time.Now()
	f := mod.ExportedFunction(call.Entrypoint)
	if f == nil {
		return mod, fmt.Errorf("could not find entrypoint function %q for module %q", call.Entrypoint, call.ModuleName)
	}
	//fmt.Println("Timing 2", time.Since(t0))

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
			ptr := writeToHeap(ctx, mod, cnt, input.Name())
			length := uint64(len(cnt))
			args = append(args, uint64(ptr), length)
		default:
			panic("unknown wasm argument type")
		}
	}

	_, err = f.Call(wasm.WithContext(ctx, call), args...)
	//defer call.deallocate(ctx, mod)
	if err != nil {
		if call.PanicError != nil {
			return mod, call.PanicError
		}
		return mod, fmt.Errorf("executing module %q: %w", call.ModuleName, err)
	}

	return mod, nil
}

//var CACHE_ENABLED = os.Getenv("WAZERO_CACHE_ENABLED") != ""

func writeToHeap(ctx context.Context, mod api.Module, data []byte, from string) uint32 {
	stack := []uint64{uint64(len(data))}
	//fmt.Println("Writing length", len(data))
	if err := mod.ExportedFunction("alloc").CallWithStack(ctx, stack); err != nil {
		panic(fmt.Errorf("alloc from %q failed: %w", from, err))
	}
	ptr := uint32(stack[0])
	if ok := mod.Memory().Write(ptr, data); !ok {
		panic("could not write to memory: " + from)
	}
	//fmt.Println("Memory size:", mod.Memory().Size())
	//if CACHE_ENABLED {
	//	c.allocations = append(c.allocations, allocation{ptr: ptr, length: uint32(len(data))})
	//}
	return ptr
}

func writeOutputToHeap(ctx context.Context, mod api.Module, outputPtr uint32, value []byte) error {
	valuePtr := writeToHeap(ctx, mod, value, "writeOutputToHeap1")
	mem := mod.Memory()
	if ok := mem.WriteUint32Le(outputPtr, valuePtr); !ok {
		panic("could not write to memory WriteUint32Le:1")
	}
	if ok := mem.WriteUint32Le(outputPtr+4, uint32(len(value))); !ok {
		panic("could not write to memory WriteUint32Le:2")
	}
	return nil
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

func addExtensionFunctions(ctx context.Context, runtime wazero.Runtime, registry *wasm.Registry) (out []wazero.CompiledModule, err error) {
	for namespace, imports := range registry.Extensions {
		builder := runtime.NewHostModuleBuilder(namespace)
		for importName, f := range imports {
			builder.NewFunctionBuilder().
				WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
					ptr, length, outputPtr := uint32(stack[0]), uint32(stack[1]), uint32(stack[2])
					data := readBytes(mod, ptr, length)
					call := wasm.FromContext(ctx)
					traceID := tracing.GetTraceID(ctx).String()

					out, err := f(ctx, traceID, call.Clock, data)
					if err != nil {
						panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, importName, err))
					}

					if ctx.Err() == context.Canceled {
						// Sometimes long-running extensions will come back to a canceled context.
						// so avoid writing to memory then
						return
					}

					if err := writeOutputToHeap(ctx, mod, outputPtr, out); err != nil {
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
