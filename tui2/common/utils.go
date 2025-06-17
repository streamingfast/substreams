package common

import (
	"github.com/jhump/protoreflect/dynamic"
	"github.com/muesli/reflow/truncate"
	networks "github.com/streamingfast/firehose-networks"
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

// InferBytesRepresentation infers the bytes representation based on the network or endpoint.
// It first checks the network ID, and if not found, it checks the endpoint.
// If neither is provided, it defaults to Hex encoding.
// It returns a dynamic.BytesRepresentation based on the encoding.
func InferBytesRepresentation(network string, endpoint string) dynamic.BytesRepresentation {
	registry := networks.GetSubstreamsRegistry()

	// First check by network and aliases
	net := registry.Find(network)
	if net == nil {
		// Try with endpoint if no network was found
		net = registry.FindBySubstreamsEndpoint(endpoint)
	}

	// If network is found, try to extract bytes representation from it
	if net != nil {
		return BytesEncodingToRepresentation(string(networks.GetBytesEncoding(net)))
	}

	return dynamic.BytesAsHex
}
