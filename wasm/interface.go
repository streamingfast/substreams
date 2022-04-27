package wasm

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WASMExtensioner interface {
	WASMExtensions() map[string]map[string]WASMExtension
}

// WASMExtension defines the implementation of a function that will
// be exposed as wasm imports; therefore, exposed to the host language
// like Rust.
//
// For example, this can be an RPC call, taking a structured request
// in `in` and outputting a structured response in `out`, both
// serialized as protobuf messages.
//
// Such a function needs to be registered through RegisterRuntime.
type WASMExtension func(ctx context.Context, request *pbsubstreams.Request, clock *pbsubstreams.Clock, in []byte) (out []byte, err error)
