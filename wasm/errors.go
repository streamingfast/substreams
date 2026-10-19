package wasm

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
)

// hostResourceErrnos are the OS errors returned when the host runs out of file
// descriptors or memory. They depend on the machine, not on the module or its
// input.
var hostResourceErrnos = []syscall.Errno{syscall.EMFILE, syscall.ENFILE, syscall.ENOMEM}

// IsHostResourceError reports whether err was caused by the host running out of
// file descriptors or memory, for example when wasmtime cannot create the memfd
// backing a new instance. Such an error can go away on retry, so it must not be
// treated as a deterministic failure of the module.
//
// wasmtime only exposes errors as text, where an OS error is rendered by Rust
// as "Too many open files (os error 24)", so the text is checked as well.
func IsHostResourceError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	for _, errno := range hostResourceErrnos {
		if errors.Is(err, errno) {
			return true
		}
		if strings.Contains(msg, fmt.Sprintf("(os error %d)", int(errno))) {
			return true
		}
	}
	return false
}
