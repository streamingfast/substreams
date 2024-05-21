package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWASMCodeType(t *testing.T) {
	tests := []struct {
		name         string
		wasmCodeType string
		want         string
		want1        RuntimeExtensions
		assertion    require.ErrorAssertionFunc
	}{
		{"plain, no extensions", "wasm/rust-v1", "wasm/rust-v1", nil, require.NoError},
		{"plain, extension wasm-bindgen-shims", "wasm/rust-v1+wasm-bindgen-shims", "wasm/rust-v1", RuntimeExtensions{
			{ID: RuntimeExtensionIDWASMBindgenShims, Value: nil},
		}, require.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParseWASMCodeType(tt.wasmCodeType)
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}
