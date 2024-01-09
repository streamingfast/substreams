package wasi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/substreams/wasm"
	"github.com/streamingfast/substreams/wasm/wasi/fs"
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
	send            io.ReadWriter
	receive         io.ReadWriter
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
	//loggerModule, err := sfwaz.AddHostFunctions(ctx, runtime, "logger", sfwaz.LoggerFuncs)
	//if err != nil {
	//	return nil, err
	//}
	//stateModule, err := sfwaz.AddHostFunctions(ctx, runtime, "state", sfwaz.StateFuncs)
	//if err != nil {
	//	return nil, err
	//}
	//hostModules = append(hostModules, loggerModule, stateModule)

	//startFunc := "main"
	//switch wasmCodeType {
	//case "go/wasi":
	//	startFunc = "_start"
	//}

	wazConfig := wazero.NewModuleConfig()

	s := bytes.NewBuffer(nil)
	r := bytes.NewBuffer(nil)

	return &Module{
		wazModuleConfig: wazConfig,
		wazRuntime:      runtime,
		userModule:      mod,
		hostModules:     hostModules,
		send:            s,
		receive:         r,
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

	return &sfwaz.Instance{}, nil
}

func (m *Module) ExecuteNewCall(ctx context.Context, call *wasm.Call, wasmInstance wasm.Instance, arguments []wasm.Argument) (out wasm.Instance, err error) {
	inst := &sfwaz.Instance{}

	argsData, err := marshallArgs(arguments)
	if err != nil {
		return nil, fmt.Errorf("marshalling args: %w", err)
	}
	fmt.Println("args data length", len(argsData))

	ctx = wasm.WithContext(sfwaz.WithInstanceContext(ctx, inst), call)
	config := m.wazModuleConfig.
		//todo: doc says this Defaults to return a deterministic source, but this does not seem to hold.  We need to investigate.
		//WithRandSource(rand.New(rand.NewSource(0))).
		WithWalltime(func() (sec int64, nsec int32) {
			t := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			return t.Unix(), int32(t.Nanosecond())
		}, sys.ClockResolution(time.Microsecond.Nanoseconds())).
		WithNanotime(func() int64 {
			return time.Since(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Nanoseconds()
		}, sys.ClockResolution(1)).
		WithSysNanosleep().
		WithStdin(m.send).
		WithStdout(m.receive).
		WithStderr(NewStdErrLogWriter(ctx)).
		WithFS(fs.NewVirtualFs(ctx)).
		WithName(call.Entrypoint).
		WithArgs(call.Entrypoint, "-inputsize", fmt.Sprintf("%d", len(argsData)))

	_, err = m.send.Write(argsData)
	if err != nil {
		return nil, fmt.Errorf("writing args: %w", err)
	}

	start := time.Now()
	if _, err := m.wazRuntime.InstantiateModule(ctx, m.userModule, config); err != nil {
		// Note: Most compilers do not exit the module after running "_start",
		// unless there was an error. This allows you to call exported functions.
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			fmt.Fprintf(os.Stderr, "exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			log.Panicln(err)
		}
	}
	fmt.Println("wazero duration", time.Since(start))

	data, err := io.ReadAll(m.receive)
	if err != nil {
		return nil, fmt.Errorf("reading output: %w", err)
	}
	call.SetReturnValue(data)

	return inst, nil
}

type message struct {
	call    string
	payload []byte
}

func (m *Module) receiveMessage(context.Context) error {
	s := bufio.NewScanner(m.receive)
	for {

		// Repeated calls to Scan yield the token sequence found in the input.
		for s.Scan() {
			encoded := s.Text()
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return fmt.Errorf("decoding input: %w", err)
			}
			msg := &message{}
			err = json.Unmarshal(decoded, msg)
			if err != nil {
				return fmt.Errorf("unmarshalling message: %w", err)
			}
			switch msg.call {
			case "Println":
				fmt.Println("printing...", string(msg.payload))
				_, err := m.send.Write([]byte("\n"))
				if err != nil {
					return fmt.Errorf("writing ok: %w", err)
				}
			default:
				panic(fmt.Errorf("unknown call: %q", msg.call))
			}

		}
		if err := s.Err(); err != nil {
			return fmt.Errorf("reading input: %w", err)
		}

	}
	return nil
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
