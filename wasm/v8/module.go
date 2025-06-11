package v8

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/proto"
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
	inputVal, _ := v8go.NewValue(inst.ctx.Isolate(), input)
	_ = inst.ctx.Global().Set("input", inputVal)

	// Runs all scripts (will be changed depending on files needed), probably going to merge all that are needed. This if makes sure we load our needed scripts ONLY on the first call
	if cachedInstance == nil {

		if err := injectAllGlobals(inst.ctx, call); err != nil {
			inst.Close(ctx)
			return nil, err
		}

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

	// call the handlers from JS side
	if err := callHandlers(inst, call); err != nil {
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

func injectAllGlobals(ctx *v8go.Context, call *wasm.Call) error {

	if err := injectStoreFunction(ctx, call); err != nil {
		return fmt.Errorf("injectStoreFunction: %w", err)
	}

	if err := injectClockFunction(ctx, call); err != nil {
		return fmt.Errorf("injectClockFunction: %w", err)
	}

	return nil
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

func callHandlers(inst *V8Instance, call *wasm.Call) error {
	handlerName := call.ModuleName

	// Check what type of handler this is
	checkTypeCode := fmt.Sprintf(`getHandlerType("%s")`, handlerName)
	handlerTypeVal, err := inst.ctx.RunScript(checkTypeCode, "check_handler_type.js")
	if err != nil {
		return fmt.Errorf("failed to check handler type: %w", err)
	}

	if handlerTypeVal.IsNull() || handlerTypeVal.IsUndefined() {
		return fmt.Errorf("handler '%s' not found", handlerName)
	}

	handlerType := handlerTypeVal.String()

	switch handlerType {
	case "map":
		return callMapHandler(inst, handlerName)
	case "store":
		return callStoreHandler(inst, call, handlerName)
	default:
		return fmt.Errorf("unknown handler type: %s", handlerType)
	}
}

func callMapHandler(inst *V8Instance, handlerName string) error {
	// Call the map handler directly, input is already set as global
	code := fmt.Sprintf(`executeMapHandler("%s", input)`, handlerName)
	result, err := inst.ctx.RunScript(code, "execute_map_handler.js")
	if err != nil {
		return fmt.Errorf("failed to execute map handler '%s': %w", handlerName, err)
	}

	return inst.ctx.Global().Set("output", result)
}

func callStoreHandler(inst *V8Instance, call *wasm.Call, handlerName string) error {
	// Call store handler with the injected store interface
	storeInterfaceCode := fmt.Sprintf(`
		executeStoreHandler("%s", {
			set: function(ordinal, key, value) {
				__store_set(ordinal, key, value);
			}
		}, output)
	`, handlerName)

	_, err := inst.ctx.RunScript(storeInterfaceCode, "execute_store_handler.js")
	return err
}

func getOutput(inst *V8Instance) ([]byte, error) {
	v8val, err := inst.ctx.Global().Get("output")
	if err != nil {
		return nil, fmt.Errorf("failed to get output global: %w", err)
	}

	if v8val.IsNull() || v8val.IsUndefined() {
		return []byte{}, nil
	}

	if !v8val.IsUint8Array() {
		return nil, fmt.Errorf("global.output is not a Uint8Array")
	}

	return v8val.Uint8Array(), nil
}

// Injects __clock into runtime
func injectClockFunction(ctx *v8go.Context, call *wasm.Call) error {
	iso := ctx.Isolate()

	clockFunc := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		clock := call.Clock

		data, err := proto.Marshal(clock)
		if err != nil {
			panic(fmt.Errorf("marshal clock: %w", err))
		}

		clockVal, err := v8go.NewUint8Array(ctx, data)
		if err != nil {
			panic(fmt.Errorf("clock Uint8Array: %w", err))
		}

		return clockVal
	})

	clockInstance := clockFunc.GetFunction(ctx)
	return ctx.Global().Set("__clock", clockInstance)
}

func injectStoreFunction(ctx *v8go.Context, call *wasm.Call) error {
	iso := ctx.Isolate()

	storeSetFunc := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		if len(info.Args()) != 3 {
			panic("__store_set expects 3 arguments")
		}

		ordinal := info.Args()[0].Integer()
		key := info.Args()[1].String()
		value := info.Args()[2]

		if !value.IsUint8Array() {
			panic("__store_set expects a Uint8Array as value")
		}

		data := value.Uint8Array()
		call.DoSet(uint64(ordinal), key, data)
		return nil
	})

	storeSetInstance := storeSetFunc.GetFunction(ctx)
	return ctx.Global().Set("__store_set", storeSetInstance)
}
