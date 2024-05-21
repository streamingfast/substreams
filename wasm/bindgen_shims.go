package wasm

const (
	WASM_BINDGEN_PLACEHOLDER_MODULE     = "__wbindgen_placeholder__"
	WASM_BINDGEN_EXTERNREF_XFORM_MODULE = "__wbindgen_externref_xform__"
)

var WASMBindgenModules = map[string]struct{}{
	WASM_BINDGEN_PLACEHOLDER_MODULE:     {},
	WASM_BINDGEN_EXTERNREF_XFORM_MODULE: {},
}
