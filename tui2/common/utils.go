package common

import (
	"github.com/jhump/protoreflect/dynamic"
	"github.com/muesli/reflow/truncate"
)

// TruncateString is a convenient wrapper around truncate.TruncateString.
func TruncateString(s string, max int) string {
	if max < 0 {
		max = 0
	}
	return truncate.StringWithTail(s, uint(max), "…")
}

// Helper to map string to dynamic.BytesRepresentation
func BytesEncodingToRepresentation(enc string) dynamic.BytesRepresentation {
	switch enc {
	case "base58":
		return dynamic.BytesAsBase58
	case "base64":
		return dynamic.BytesAsBase64
	case "string":
		return dynamic.BytesAsString
	default:
		return dynamic.BytesAsHex
	}
}
