package wasmtime

import (
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v41"
)

// registerContextImports exposes the execution context a module runs in. Unlike the
// `state` getters the value always exists, so the import returns nothing and simply
// writes the payload (a `{ptr, len}` pair) at `outputPtr`.
func (i *instance) registerContextImports(linker *wasmtime.Linker) error {
	if err := linker.FuncWrap("context", "clock",
		func(outputPtr int32) {
			if err := writeOutputToHeap(i, outputPtr, i.CurrentCall.DoClock()); err != nil {
				i.CurrentCall.PanicDeterministicError("writing clock to heap: %w", err)
			}
		},
	); err != nil {
		return fmt.Errorf("registering clock import: %w", err)
	}

	return nil
}
