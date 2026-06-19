package wasmtime

import (
	"context"
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v41"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
)

type Module struct {
	module            *wasmtime.Module
	engine            *wasmtime.Engine
	registry          *wasm.Registry
	runtimeExtensions wasm.RuntimeExtensions
}

func init() {
	wasm.RegisterModuleFactory("wasmtime", wasm.ModuleFactoryFunc(newModule))
}

func newModule(ctx context.Context, wasmCode []byte, wasmCodeType string, registry *wasm.Registry) (wasm.Module, error) {
	cfg := wasmtime.NewConfig()

	// disabled because 50% performance hit in some cases
	// cfg.SetConsumeFuel(true)

	engine := wasmtime.NewEngineWithConfig(cfg)

	wasmCodeTypeID, runtimeExtensions, err := wasm.ParseWASMCodeType(wasmCodeType)
	if err != nil {
		return nil, fmt.Errorf("invalid wasm code type %q: %w", wasmCodeType, err)
	}

	// For now, we only support the basic wasmCodeTypeID without specific handling
	// TODO: Add support for different wasmCodeTypeID variants if needed
	_ = wasmCodeTypeID

	module, err := wasmtime.NewModule(engine, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("creating new module: %w", err)
	}

	// TODO: IF POSSIBLE, hook up all the wasm imports at this point, not at
	// instantiation time.

	return &Module{
		module:            module,
		engine:            engine,
		registry:          registry,
		runtimeExtensions: runtimeExtensions,
	}, nil
}

func (m *Module) Close(_ context.Context) error {
	m.engine.Close()
	return nil
}

func (m *Module) NewInstance(ctx context.Context) (instance wasm.Instance, err error) {
	inst, err := m.newInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not instantiate wasm module: %w", err)
	}

	return inst, nil
}

func (m *Module) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument, values map[string][]byte) (returnInstance wasm.Instance, err error) {
	var inst *instance
	if cachedInstance != nil {
		inst = cachedInstance.(*instance)
		if inst.isClosed {
			panic("module is closed")
		}
	} else {
		inst, err = m.newInstance(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not instantiate wasm module: %w", err)
		}
		defer func() {
			if e := inst.Close(ctx); e != nil {
				reqctx.Logger(ctx).Warn("cannot close instance", zap.Error(e))
				if err == nil {
					err = e
				}
			}
		}()
	}

	export := inst.wasmInstance.GetExport(inst.wasmStore, call.Entrypoint)
	if export == nil {
		return nil, fmt.Errorf("failed to get entrypoint %q", call.Entrypoint)
	}
	entrypoint := export.Func()
	if entrypoint == nil {
		return nil, fmt.Errorf("failed to get exported function %T", entrypoint)
	}

	var args []interface{}
	var inputStoreCount int
	var foundationalStoreCount int

	for _, input := range arguments {
		switch v := input.(type) {
		case *wasm.StoreWriterOutput:
		case *wasm.StoreReaderInput:
			inputStoreCount++
			args = append(args, int32(inputStoreCount-1))
		case *wasm.ParamsInput:
			cnt := v.Value()
			ptr, err := inst.Heap.Write(cnt, input.Name())
			if err != nil {
				return nil, fmt.Errorf("writing %s to heap: %w", input.Name(), err)
			}
			length := int32(len(cnt))
			args = append(args, ptr, length)
		case *wasm.MapInput, *wasm.StoreDeltaInput, *wasm.SourceInput:
			cnt := values[v.Name()]
			ptr, err := inst.Heap.Write(cnt, input.Name())
			if err != nil {
				return nil, fmt.Errorf("writing %s to heap: %w", input.Name(), err)
			}
			length := int32(len(cnt))
			args = append(args, ptr, length)
		case *wasm.FoundationalStoreInput:
			args = append(args, int32(foundationalStoreCount))
			foundationalStoreCount++
		default:
			panic("unknown wasm argument type")
		}
	}

	inst.CurrentCall = call
	_, err = entrypoint.Call(inst.wasmStore, args...)
	if err != nil {
		return inst, fmt.Errorf("calling entrypoint: %w", err)
	}

	return inst, nil
}

func (m *Module) newInstance(ctx context.Context) (*instance, error) {
	linker := wasmtime.NewLinker(m.engine)
	store := wasmtime.NewStore(m.engine)
	// disabled because 50% performance hit in some cases
	// err := store.SetFuel(5_000_000_000_000)

	i := &instance{
		wasmEngine: m.engine,
		wasmLinker: linker,
		wasmStore:  store,
		wasmModule: m.module,
	}
	if err := i.newImports(); err != nil {
		return nil, fmt.Errorf("instantiating imports: %w", err)
	}
	for namespace, imports := range m.registry.Extensions {
		for importName, f := range imports {
			f := i.newExtensionFunction(ctx, namespace, importName, f)
			if err := linker.FuncWrap(namespace, importName, f); err != nil {
				return nil, fmt.Errorf("instantiating %q extension import: %w", namespace, err)
			}
		}
	}

	// Handle wasm-bindgen-shims if enabled
	if m.runtimeExtensions.Has(wasm.RuntimeExtensionIDWASMBindgenShims) {
		if err := m.addWASMBindgenShims(ctx, linker, store); err != nil {
			return nil, fmt.Errorf("adding wasm-bindgen shims: %w", err)
		}
	}

	instance, err := i.wasmLinker.Instantiate(i.wasmStore, i.wasmModule)
	if err != nil {
		return nil, fmt.Errorf("creating new instance: %w", err)
	}
	memory := instance.GetExport(i.wasmStore, "memory").Memory()
	alloc := instance.GetExport(i.wasmStore, "alloc").Func()
	dealloc := instance.GetExport(i.wasmStore, "dealloc").Func()
	if alloc == nil || dealloc == nil {
		panic("missing malloc or free")
	}

	heap := NewHeap(memory, alloc, dealloc, i.wasmStore)
	i.Heap = heap
	i.wasmInstance = instance
	return i, nil
}

// addWASMBindgenShims adds shim functions for wasm-bindgen modules
func (m *Module) addWASMBindgenShims(_ context.Context, linker *wasmtime.Linker, store *wasmtime.Store) error {
	// Gather unbound imports to see what we actually need to shim
	unboundedImports := m.gatherUnboundedModuleImports(linker, store)

	for moduleName, imports := range unboundedImports {
		_, found := wasm.WASMBindgenModules[moduleName]
		if !found {
			// We only shim bindgen modules, it will fail later since this module we skip is unbound
			continue
		}

		if err := m.addShimModule(linker, store, moduleName, imports); err != nil {
			return fmt.Errorf("adding shim module %q: %w", moduleName, err)
		}
	}
	return nil
}

// addShimModule adds a shim module with dummy functions that panic when called
func (m *Module) addShimModule(linker *wasmtime.Linker, store *wasmtime.Store, moduleName string, imports []*wasmtime.ImportType) error {
	for _, imp := range imports {
		funcName := imp.Name()
		if funcName == nil {
			continue
		}

		funcType := imp.Type().FuncType()
		if funcType == nil {
			continue
		}

		// Create a generic shim function that matches any signature and panics when called
		shimFunc := wasmtime.NewFunc(store, funcType, func(*wasmtime.Caller, []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
			panic(fmt.Errorf("you are trying to call %q from module %q for which Substreams provided a dummy"+
				" implementation, you are allowed to have it referenced but it's forbidden to call it since"+
				" we cannot provide a real implementation for it", *funcName, moduleName))
		})

		if err := linker.Define(store, moduleName, *funcName, shimFunc); err != nil {
			return fmt.Errorf("defining shim function %q in module %q: %w", *funcName, moduleName, err)
		}
	}

	return nil
}

// gatherUnboundedModuleImports returns a map of module names to their unbound imports
func (m *Module) gatherUnboundedModuleImports(linker *wasmtime.Linker, store *wasmtime.Store) map[string][]*wasmtime.ImportType {
	importsByModule := make(map[string][]*wasmtime.ImportType)

	for _, imp := range m.module.Imports() {
		moduleName := imp.Module()

		// Only process function imports since we're creating function shims
		if imp.Type().FuncType() == nil {
			continue
		}

		// Check if this import is already bound in the linker
		if imp.Name() != nil {
			if existing := linker.Get(store, imp.Module(), *imp.Name()); existing != nil {
				continue
			}
		}

		importsByModule[moduleName] = append(importsByModule[moduleName], imp)
	}

	return importsByModule
}
