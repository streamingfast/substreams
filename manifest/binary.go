package manifest

import "strings"

// SplitBinaryType splits a binary type in two components: the type ID and the raw extensions.
//
// The format is `<id>+<extensions>`, where `<id>` is the binary type ID and `<extensions>`
// is a comma-separated list of runtime extensions that are type specific. The `+<extensions>` part
// is optional and can be omitted.
//
// So we accept the following formats:
//
//   - wasm/rust-v1
//   - wasm/rust-v1+wasm-bindgen-shims
//   - wasm/rust-v1+wasm-bindgen-shims,other-extension=value
//
// This method returns the ID and the raw unsplitted extensions string.
// The input `wasm/rust-v1+wasm-bindgen-shims,other-extension=value` would
// result in ("wasm/rust-v1", "wasm-bindgen-shims,other-extension=value") being
// returned to your
func SplitBinaryType(in string) (typeID string, rawExtensions string) {
	typeID, rawExtensions, _ = strings.Cut(in, "+")
	return
}
