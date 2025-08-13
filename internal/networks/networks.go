package networks

// GetSubstreamsRegistry returns a map of network names to their endpoints
func GetSubstreamsRegistry() map[string]string {
	return map[string]string{
		"mainnet": "mainnet.eth.streamingfast.io:443",
	}
}

// GetSubstreamsEndpoint returns the endpoint for a given network
func GetSubstreamsEndpoint(network string) string {
	registry := GetSubstreamsRegistry()
	if endpoint, ok := registry[network]; ok {
		return endpoint
	}
	return network
}

// GetBytesEncoding returns the bytes encoding for a given network
func GetBytesEncoding(network string) []byte {
	return []byte("hex")
}

