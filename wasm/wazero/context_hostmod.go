package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/streamingfast/substreams/wasm"
)

// ContextFuncs exposes the execution context a module runs in. Unlike the `state` getters
// the value always exists, so the import returns nothing and simply writes the payload
// (a `{ptr, len}` pair) at `output_ptr`.
var ContextFuncs = []funcs{
	{
		"clock",
		[]parm{i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			outputPtr := uint32(stack[0])
			call := wasm.FromContext(ctx)
			inst := instanceFromContext(ctx)

			if err := writeOutputToHeap(ctx, inst, outputPtr, call.DoClock()); err != nil {
				call.PanicDeterministicError("writing clock to heap: %w", err)
			}
		}),
	},
}
