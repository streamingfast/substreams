package wasi

import (
	"fmt"

	"github.com/protocolbuffers/protoscope"
	"github.com/streamingfast/substreams/wasm"
)

func marshallArgs(args []wasm.Argument) ([]byte, error) {
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
			scopeData += fmt.Sprintf("%d: %s\n", fieldCount, v.ProtoScopeValue())
		case *wasm.SourceInput:
			scopeData += fmt.Sprintf("%d: %s\n", fieldCount, v.ProtoScopeValue())
		default:
			panic(fmt.Sprintf("unknown wasm argument type %T", v))
		}
	}

	return protoscope.NewScanner(scopeData).Exec()

}
