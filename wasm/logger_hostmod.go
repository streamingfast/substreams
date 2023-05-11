package wasm

import (
	"context"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/tetratelabs/wazero/api"
	"go.uber.org/zap"
)

var loggerFuncs = []funcs{
	{
		"println",
		[]parm{i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ptr, length := uint32(stack[0]), uint32(stack[1])
			call := fromContext(ctx)

			if call.ReachedLogsMaxByteCount() {
				// Early exit, we don't even need to collect the message as we would not store it anyway
				return
			}

			if length > maxLogByteCount {
				panic(fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length))))
			}

			message := readString(mod, ptr, length)
			if tracer.Enabled() {
				zlog.Debug(message, zap.String("module_name", call.moduleName))
			}

			// len(<string>) in Go count number of bytes and not characters, so we are good here
			call.LogsByteCount += uint64(len(message))
			if !call.ReachedLogsMaxByteCount() {
				call.Logs = append(call.Logs, message)
				call.ExecutionStack = append(call.ExecutionStack, fmt.Sprintf("log: %s", message))
			}
			return
		}),
	},
}
