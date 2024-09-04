package wasi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/protocolbuffers/protoscope"
	"github.com/streamingfast/substreams/wasm"
	"github.com/streamingfast/substreams/wasm/wasi/fs"
	sfwazero "github.com/streamingfast/substreams/wasm/wazero"
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
	send            io.ReadWriter
}

func init() {
	// TODO: we'd need to hook that into the list of "BinaryTypes" so it can be enabled on a per-request basis
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

	var hostModules []wazero.CompiledModule
	wazConfig := wazero.NewModuleConfig()

	s := bytes.NewBuffer(nil)

	return &Module{
		wazModuleConfig: wazConfig,
		wazRuntime:      runtime,
		userModule:      mod,
		hostModules:     hostModules,
		send:            s,
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

	return sfwazero.NewInstance(nil, sfwazero.TinyGoSauce), nil
}

func (m *Module) ExecuteNewCall(ctx context.Context, call *wasm.Call, wasmInstance wasm.Instance, arguments []wasm.Argument, argumentValues map[string][]byte) (out wasm.Instance, err error) {
	inst := sfwazero.NewInstance(nil, sfwazero.TinyGoSauce)

	argsData, err := marshallArgs(arguments, argumentValues)
	if err != nil {
		return nil, fmt.Errorf("marshalling args: %w", err)
	}

	ctx = wasm.WithContext(sfwazero.WithInstanceContext(ctx, inst), call)
	vfs := fs.NewVirtualFs(ctx)
	logWriter := NewLogWriter(ctx)

	config := m.wazModuleConfig.
		WithRandSource(rand.New(rand.NewSource(42))).
		WithStdin(m.send).
		WithStdout(logWriter).
		WithStderr(logWriter).
		WithFS(vfs).
		WithName(call.Entrypoint).
		WithArgs(call.Entrypoint, "--inputsize", fmt.Sprintf("%d", len(argsData)))

	_, err = m.send.Write(argsData)
	if err != nil {
		return nil, fmt.Errorf("writing args: %w", err)
	}

	if _, err := m.wazRuntime.InstantiateModule(ctx, m.userModule, config); err != nil {
		// Note: Most compilers do not exit the module after running "_start",
		// unless there was an error. This allows you to call exported functions.
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			fmt.Fprintf(os.Stderr, "exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			log.Panicln(fmt.Errorf("error in InstantiateModule: %w", err))
		}
	}

	return inst, nil
}

type message struct {
	call    string
	payload []byte
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

type LogWriter struct {
	mu  sync.Mutex
	ctx context.Context
}

func NewLogWriter(ctx context.Context) io.Writer {
	return &LogWriter{
		ctx: ctx,
	}
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	logMessage := string(p)
	length := len(p)

	call := wasm.FromContext(w.ctx)

	if call.ReachedLogsMaxByteCount() {
		// Early exit, we don't even need to collect the message as we would not store it anyway
		return 0, nil
	}

	if length > wasm.MaxLogByteCount {
		return 0, fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length)))
	}

	call.AppendLog(logMessage)
	return len(p), nil
}

func marshallArgs(args []wasm.Argument, values map[string][]byte) ([]byte, error) {
	scopeData := ""
	fieldCount := 0
	writeStoreCount := 0
	readerStoreCount := 0
	for _, arg := range args {
		fieldCount++
		switch v := arg.(type) {
		case *wasm.StoreWriterOutput:
			scopeData += fmt.Sprintf("%d: %d\n", fieldCount, writeStoreCount)
			writeStoreCount++
		case *wasm.StoreReaderInput:
			scopeData += fmt.Sprintf("%d: %d\n", fieldCount, readerStoreCount)
			readerStoreCount++
		case wasm.ProtoScopeValueArgument:
			val := values[arg.Name()]
			scopeData += fmt.Sprintf("%d: %s\n", fieldCount, v.ProtoScopeValue(val))
		default:
			panic(fmt.Sprintf("unknown wasm argument type %T", v))
		}
	}
	data, err := protoscope.NewScanner(scopeData).Exec()
	if err != nil {
		return nil, fmt.Errorf("scanning args: %w (scopeData: %s)", err, scopeData)
	}
	return data, nil
}
