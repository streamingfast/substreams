package wazero

import (
	"context"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/substreams/wasm"
	"github.com/tetratelabs/wazero/api"
	"go.uber.org/zap"
)

var LoggerFuncs = []funcs{
	{
		"println",
		[]parm{i32, i32}, // ptr, len
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			message := readStringFromStack(mod, stack[0:])
			length := uint32(stack[1])
			call := wasm.FromContext(ctx)
			//fmt.Println("IN HEREEEEEEEEEEEEEEEEEEEEEEEEEEEEE", stack, call.ReachedLogsMaxByteCount(), message)

			if call.ReachedLogsMaxByteCount() {
				// Early exit, we don't even need to collect the message as we would not store it anyway
				return
			}

			if length > wasm.MaxLogByteCount {
				panic(fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length))))
			}

			if tracer.Enabled() {
				zlog.Debug(message, zap.String("module_name", call.ModuleName), zap.String("log_source", "wasm"))
			}
			call.AppendLog(message)
		}),
	},
}
