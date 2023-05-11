package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

var envFuncs = []funcs{
	{
		"register_panic",
		[]parm{i32, i32, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			message := readStringFromStack(mod, stack[0:])
			lineNo, colNo := uint32(stack[4]), uint32(stack[5])
			var filename string
			if filePtr := stack[2]; filePtr != 0 {
				filename = readStringFromStack(mod, stack[2:])
			}

			call := fromContext(ctx)

			call.panicError = &PanicError{message, filename, int(lineNo), int(colNo)}
		}),
	},
	{
		"output",
		[]parm{i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			call := fromContext(ctx)
			msg := readBytesFromStack(mod, stack[0:])
			call.returnValue = make([]byte, uint32(stack[1]))
			copy(call.returnValue, msg)
		}),
	},
}
