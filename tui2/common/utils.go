package common

import (
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/substreams/sink"
)

// ToDynamicBytesRepresentation converts a sink.BytesRepresentation to a dynamic.BytesRepresentation.
// We don't use dynamic.BytesRepresentation directly because it requires a golang override of dynamic module
func ToDynamicBytesRepresentation(bytesRepresentation sink.BytesRepresentation) dynamic.BytesRepresentation {
	switch bytesRepresentation {
	case sink.BytesAsBase58:
		return dynamic.BytesAsBase58
	case sink.BytesAsBase64:
		return dynamic.BytesAsBase64
	case sink.BytesAsString:
		return dynamic.BytesAsString
	case sink.BytesAsHex:
		return dynamic.BytesAsHex
	default:
		return dynamic.BytesAsHex
	}
}
