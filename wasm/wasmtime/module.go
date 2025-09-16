package wasmtime

import (
	"context"
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v30"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
)

type Module struct {
	module   *wasmtime.Module
	engine   *wasmtime.Engine
	registry *wasm.Registry
}

func init() {
	wasm.RegisterModuleFactory("wasmtime", wasm.ModuleFactoryFunc(newModule))
}

func newModule(ctx context.Context, wasmCode []byte, wasmCodeType string, registry *wasm.Registry) (wasm.Module, error) {
	cfg := wasmtime.NewConfig()

	// disabled because 50% performance hit in some cases
	// cfg.SetConsumeFuel(true)

	engine := wasmtime.NewEngineWithConfig(cfg)

	module, err := wasmtime.NewModule(engine, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("creating new module: %w", err)
	}

	// TODO: IF POSSIBLE, hook up all the wasm imports at this point, not at
	// instantiation time.

	return &Module{
		module:   module,
		engine:   engine,
		registry: registry,
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
		return nil, fmt.Errorf("failed to get exported function %q", entrypoint)
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
