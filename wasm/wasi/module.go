package wasi

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	sfwaz "github.com/streamingfast/substreams/wasm/wazero"

	"github.com/streamingfast/substreams/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// A Module represents a wazero.Runtime that clears and is destroyed upon completion of a request.
// It has the pre-compiled `env` host module, as well as pre-compiled WASM code provided by the user
type Module struct {
	sync.Mutex
	wazRuntime      wazero.Runtime
	wazModuleConfig wazero.ModuleConfig
	userModule      wazero.CompiledModule
	hostModules     []wazero.CompiledModule
}

func init() {
	wasm.RegisterModuleFactory("wasi", wasm.ModuleFactoryFunc(newModule))
}

func newModule(ctx context.Context, wasmCode []byte, wasmCodeType string, registry *wasm.Registry) (wasm.Module, error) {
	runtimeConfig := wazero.NewRuntimeConfigCompiler()
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	mod, err := runtime.CompileModule(ctx, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("creating new module: %w", err)
	}

	hostModules := []wazero.CompiledModule{}
	loggerModule, err := sfwaz.AddHostFunctions(ctx, runtime, "logger", sfwaz.LoggerFuncs)
	if err != nil {
		return nil, err
	}
	hostModules = append(hostModules, loggerModule)

	//startFunc := "main"
	//switch wasmCodeType {
	//case "go/wasi":
	//	startFunc = "_start"
	//}

	wazConfig := wazero.NewModuleConfig().WithStderr(os.Stderr)
	//WithStartFunctions(startFunc)

	return &Module{
		wazModuleConfig: wazConfig,
		wazRuntime:      runtime,
		userModule:      mod,
		hostModules:     hostModules,
	}, nil
}

func (m *Module) Close(ctx context.Context) error {
	return nil
}

func (m *Module) NewInstance(ctx context.Context) (out wasm.Instance, err error) {
	err = m.instantiateModule(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not instantiate wasm module: %w", err)
	}

	return &instance{}, nil
}

func (m *Module) ExecuteNewCall(ctx context.Context, call *wasm.Call, wasmInstance wasm.Instance, arguments []wasm.Argument) (out wasm.Instance, err error) {
	inst := &instance{}
	sourceInput := arguments[0].(*wasm.SourceInput)

	//todo: on to access exported functions if go wasi
	//f := mod.ExportedFunction("run")
	//if f == nil {
	//	return inst, fmt.Errorf("could not find entrypoint function %q ", call.Entrypoint)
	//}

	//arguments ->
	//message MapPoolCreatedInput {
	//	string params = 1;
	//	sf.ethereum.type.v2.Block block = 2;
	//	sf.substreams.type.v1.Clock clock = 3;
	//	uniswap.v3.Swaps = 4;
	//	uint32 token_store_reader_id = 5;
	//	uint32 mymod_writer_id = 6;
	//}

	r := bytes.NewReader(sourceInput.Value())
	w := bytes.NewBuffer(nil)
	config := m.wazModuleConfig.WithStdin(r).WithStdout(w).WithArgs()

	ctx2 := wasm.WithContext(withInstanceContext(ctx, inst), call)
	if _, err := m.wazRuntime.InstantiateModule(ctx2, m.userModule, config); err != nil {
		// Note: Most compilers do not exit the module after running "_start",
		// unless there was an error. This allows you to call exported functions.
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			fmt.Fprintf(os.Stderr, "exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			log.Panicln(err)
		}
	}
	call.SetReturnValue(w.Bytes())

	return inst, nil
}

func (m *Module) instantiateModule(ctx context.Context) error {
	m.Lock()
	defer m.Unlock()

	for _, hostMod := range m.hostModules {
		if m.wazRuntime.Module(hostMod.Name()) != nil {
			continue
		}
		_, err := m.wazRuntime.InstantiateModule(ctx, hostMod, m.wazModuleConfig.WithName(hostMod.Name()))
		if err != nil {
			return fmt.Errorf("instantiating host module %q: %w", hostMod.Name(), err)
		}
	}
	return nil
}
