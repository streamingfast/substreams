package wasm

import (
	"fmt"
)

type PanicError struct {
	message      string
	filename     string
	lineNumber   int
	columnNumber int
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic in the wasm: %q at %s:%d:%d", e.message, e.filename, e.lineNumber, e.columnNumber)
}
