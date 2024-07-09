//go:build wasi

package main

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/streamingfast/tinygo-test/pb"
)

//go:generate substreams protogen ./substreams.yaml --with-tinygo-maps // creates genre substreams.gen.go

// Dans WASI: _start
func main() {}

//export db_get_i64
func _db_get_i64(code, scope, key uint64) []byte

//export output
func _output(ptr, len uint32)

//go:wasm-module logger
//export println
func _log(ptr, len uint32)

// Output the serialized protobuf byte array to the Substreams engine
func output(out []byte) {
	_output(byteArrayToPtr(out))
}

// Log a line to the Substreams engine
func logf(message string, args ...any) {
	_log(stringToPtr(fmt.Sprintf(message, args...)))
}

//export map_test
func _map_test(blockPtr, blockLen uint32) (retval uint32) {
	defer func() {
		if r := recover(); r != nil {
			logf("%#v", r)
			retval = 1
		}
	}()

	a := ptrToString(blockPtr, blockLen)
	b := []byte(a)
	dest := &pb.Block{}
	if err := dest.UnmarshalVT(b); err != nil {
		logf("failed unmarshal: %w, %d", err, len(a), len(b), b[:20])
		return 1
		//panic(fmt.Errorf("failed unmarshal: %w", err))
	}

	ret, err := map_test(dest)
	if err != nil {
		panic(fmt.Errorf("map_test failed: %w", err))
	}
	if ret != nil {
		cnt, err := ret.MarshalVT()
		if err != nil {
			panic(fmt.Errorf("marshal output: %w", err))
		}
		output(cnt)
	}
	return 0
}

// ptrToString returns a string from WebAssembly compatible numeric types
// representing its pointer and length.
func ptrToString(ptr uint32, size uint32) string {
	// Get a slice view of the underlying bytes in the stream. We use SliceHeader, not StringHeader
	// as it allows us to fix the capacity to what was allocated.
	return *(*string)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(ptr),
		Len:  uintptr(size), // Tinygo requires these as uintptrs even if they are int fields.
		Cap:  uintptr(size), // ^^ See https://github.com/tinygo-org/tinygo/issues/1284
	}))
}

// stringToPtr returns a pointer and size pair for the given string in a way
// compatible with WebAssembly numeric types.
func stringToPtr(s string) (uint32, uint32) {
	buf := []byte(s)
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return uint32(unsafePtr), uint32(len(buf))
}

// byteArrayToPtr returns a pointer and size pair for the given byte array, for WASM compat.
func byteArrayToPtr(buf []byte) (uint32, uint32) {
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return uint32(unsafePtr), uint32(len(buf))
}
