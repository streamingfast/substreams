package wasi

import (
	"bytes"
	"context"
	"fmt"
	"github.com/dustin/go-humanize"
	"io"
	"log"
	"os"
	"sync"

	"github.com/streamingfast/substreams/wasm"
	sfwaz "github.com/streamingfast/substreams/wasm/wazero"
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
	stateModule, err := sfwaz.AddHostFunctions(ctx, runtime, "state", sfwaz.StateFuncs)
	if err != nil {
		return nil, err
	}
	hostModules = append(hostModules, loggerModule, stateModule)

	//startFunc := "main"
	//switch wasmCodeType {
	//case "go/wasi":
	//	startFunc = "_start"
	//}

	wazConfig := wazero.NewModuleConfig()

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

	argsData, err := marshallArgs(arguments)
	if err != nil {
		return nil, fmt.Errorf("marshalling args: %w", err)
	}

	r := bytes.NewReader(argsData)
	w := bytes.NewBuffer(nil)
	ctx = wasm.WithContext(withInstanceContext(ctx, inst), call)

	config := m.wazModuleConfig.WithStdin(r).WithStdout(w).WithStderr(NewStdErrLogWriter(ctx)).WithArgs("mapBlock")

	if _, err := m.wazRuntime.InstantiateModule(ctx, m.userModule, config); err != nil {
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

type StdErrLogWriter struct {
	ctx context.Context
}

func NewStdErrLogWriter(ctx context.Context) io.Writer {
	return &StdErrLogWriter{
		ctx: ctx,
	}
}

func (w *StdErrLogWriter) Write(p []byte) (n int, err error) {
	message := string(p)
	length := len(p)

	call := wasm.FromContext(w.ctx)

	if call.ReachedLogsMaxByteCount() {
		// Early exit, we don't even need to collect the message as we would not store it anyway
		return 0, nil
	}

	if length > wasm.MaxLogByteCount {
		return 0, fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length)))
	}

	call.AppendLog(message)
	return len(p), nil
}
