package v8

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/streamingfast/substreams/wasm"
	"rogchap.com/v8go"
)

//go:embed runtime/polyfill.bundle.js
var polyfillCode string

type V8Module struct {
	iso      *v8go.Isolate
	code     []byte
	registry *wasm.Registry
}

func (mod *V8Module) NewInstance(context.Context) (wasm.Instance, error) {
	return NewV8Instance(mod.iso)
}

func (mod *V8Module) ExecuteNewCall(
	ctx context.Context,
	call *wasm.Call,
	cachedInstance wasm.Instance,
	_ []wasm.Argument,
	argValues map[string][]byte,
) (wasm.Instance, error) {

	// Used to inject input, entry buffer
	var input []byte
	for _, val := range argValues {
		input = val
		break
	}
	if input == nil {
		input = []byte{}
	}

	inst := getInstance(mod.iso, cachedInstance)

	// Inject input before running scripts (needed to run main)
	if err := runJS(inst, InjectUint8Array("input", input), "inject_input.js"); err != nil {
		inst.Close(ctx)
		return nil, err
	}

	// Runs all scripts (will be changed depending on files needed), probably going to merge all that are needed
	// this if makes sure we load our needed scripts ONLY on the first call
	if cachedInstance == nil {
		scripts := []struct{ code, name string }{
			{polyfillCode, "polyfill.js"},
			{string(mod.code), "bundle.js"},
		}
		for _, s := range scripts {
			if err := runJS(inst, s.code, s.name); err != nil {
				inst.Close(ctx)
				return nil, err
			}
		}
	}

	// call the actual js function
	if err := callMain(inst); err != nil {
		inst.Close(ctx)
		return nil, err
	}

	outBytes, err := getOutput(inst)
	_ = inst.ctx.Global().Set("output", v8go.Null(inst.ctx.Isolate()))

	if err != nil {
		inst.Close(ctx)
		return nil, err
	}

	call.SetReturnValue(outBytes)
	return inst, nil
}

func (mod *V8Module) Close(context.Context) error {
	mod.iso.Dispose()
	return nil
}

func getInstance(iso *v8go.Isolate, cachedInstance wasm.Instance) *V8Instance {
	if cachedInstance != nil {
		return cachedInstance.(*V8Instance)
	}
	v8, _ := NewV8Instance(iso)
	return v8
}

func runJS(inst *V8Instance, code, filename string) error {
	if _, err := inst.ctx.RunScript(code, filename); err != nil {
		return fmt.Errorf("executing %s: %w", filename, err)
	}
	return nil
}

func callMain(inst *V8Instance) error {

	// A value in v8go is a type from the lib used internally
	v8val, _ := inst.ctx.Global().Get("main")
	if !v8val.IsFunction() {
		return fmt.Errorf("global.main is not a function")
	}

	// Converts js value -> v8go function
	fn, _ := v8val.AsFunction()

	// call(undefined) the main function without arguments so main() is executed
	_, err := fn.Call(v8go.Undefined(inst.ctx.Isolate()))
	return err
}

// Returns bytes from Uint8Array
func getOutput(inst *V8Instance) ([]byte, error) {
	v8val, _ := inst.ctx.Global().Get("output")
	if !v8val.IsObject() {
		return nil, fmt.Errorf("global.output is not an object")
	}

	return ExtractUint8Array(v8val.Object())
}
