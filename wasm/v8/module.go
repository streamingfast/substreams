package v8

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/streamingfast/substreams/wasm"
	"rogchap.com/v8go"
)

type V8Module struct {
	iso      *v8go.Isolate
	code     []byte
	registry *wasm.Registry
}

//go:embed runtime/polyfill.bundle.js
var polyfillCode string

//go:embed runtime/prelude.bundle.js
var preludeCode string

func (mod *V8Module) NewInstance(ctx context.Context) (wasm.Instance, error) {
	return NewV8Instance(mod.iso)
}

func (mod *V8Module) ExecuteNewCall(
	ctx context.Context,
	call *wasm.Call,
	cachedInstance wasm.Instance,
	arguments []wasm.Argument,
	argValues map[string][]byte,
) (wasm.Instance, error) {
	var inst *V8Instance
	var err error

	if cachedInstance != nil {
		inst = cachedInstance.(*V8Instance)
	} else {
		inst, err = NewV8Instance(mod.iso)
		if err != nil {
			return nil, fmt.Errorf("creating new V8 instance: %w", err)
		}
		defer func() {
			if err := inst.Close(ctx); err != nil {
				fmt.Printf("error closing V8 instance: %s\n", err)
			}
		}()
	}

	if _, err = inst.ctx.RunScript(polyfillCode, "polyfill.js"); err != nil {
		inst.Close(ctx)
		return nil, fmt.Errorf("executing polyfill: %w", err)
	}

	if _, err = inst.ctx.RunScript(preludeCode, "prelude.js"); err != nil {
		inst.Close(ctx)
		return nil, fmt.Errorf("executing prelude: %w", err)
	}

	if _, err = inst.ctx.RunScript(string(mod.code), "bundle.js"); err != nil {
		inst.Close(ctx)
		return nil, fmt.Errorf("executing JS bundle: %w", err)
	}

	defer func() { inst = nil }()

	return inst, nil
}

func (mod *V8Module) Close(ctx context.Context) error {
	mod.iso.Dispose()
	return nil
}
