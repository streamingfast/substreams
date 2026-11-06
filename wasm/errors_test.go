package wasm

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsHostResourceError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil", nil, false},
		{"wasmtime memfd out of file descriptors", errors.New("could not instantiate wasm module: creating new instance: cannot create a memfd: Too many open files (os error 24)"), true},
		{"wasmtime system file table full", errors.New("creating new instance: Too many open files in system (os error 23)"), true},
		{"wasmtime mmap out of memory", errors.New("creating new instance: failed to create stack pool mapping: Cannot allocate memory (os error 12)"), true},
		{"go wrapped EMFILE", fmt.Errorf("could not instantiate wasm module: %w", &os.PathError{Op: "open", Path: "/tmp/x", Err: syscall.EMFILE}), true},
		{"go wrapped ENOMEM", fmt.Errorf("instantiating: %w", syscall.ENOMEM), true},
		{"wasmtime unknown import", errors.New("could not instantiate wasm module: creating new instance: unknown import: `env::missing` has not been defined"), false},
		{"wazero unknown import", errors.New("could not instantiate wasm module: module[missing] not instantiated"), false},
		{"trap during execution", errors.New("calling entrypoint: wasm trap: wasm `unreachable` instruction executed"), false},
		{"other os error", errors.New("open foo: No such file or directory (os error 2)"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsHostResourceError(tt.err))
		})
	}
}
