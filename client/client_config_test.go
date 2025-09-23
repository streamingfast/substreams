package client

import (
	"testing"
)

func TestNewSubstreamsClientConfig(t *testing.T) {
	tests := []struct {
		name              string
		endpoint          string
		expectedEndpoint  string
		expectedPlaintext bool
	}{
		{
			name:              "http without port should add :80",
			endpoint:          "http://example.com",
			expectedEndpoint:  "example.com:80",
			expectedPlaintext: true,
		},
		{
			name:              "http with port should keep the port",
			endpoint:          "http://example.com:8080",
			expectedEndpoint:  "example.com:8080",
			expectedPlaintext: true,
		},
		{
			name:              "https without port should add :443",
			endpoint:          "https://example.com",
			expectedEndpoint:  "example.com:443",
			expectedPlaintext: false,
		},
		{
			name:              "https with port should keep the port",
			endpoint:          "https://example.com:9443",
			expectedEndpoint:  "example.com:9443",
			expectedPlaintext: false,
		},
		{
			name:              "plain endpoint without scheme should remain unchanged",
			endpoint:          "example.com:443",
			expectedEndpoint:  "example.com:443",
			expectedPlaintext: false,
		},
		{
			name:              "IP address http without port should add :80",
			endpoint:          "http://192.168.1.1",
			expectedEndpoint:  "192.168.1.1:80",
			expectedPlaintext: true,
		},
		{
			name:              "IP address https without port should add :443",
			endpoint:          "https://192.168.1.1",
			expectedEndpoint:  "192.168.1.1:443",
			expectedPlaintext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewSubstreamsClientConfig(tt.endpoint, "", None, false, false, "test-agent")

			if config.endpoint != tt.expectedEndpoint {
				t.Errorf("expected endpoint %q, got %q", tt.expectedEndpoint, config.endpoint)
			}

			if config.plaintext != tt.expectedPlaintext {
				t.Errorf("expected plaintext %v, got %v", tt.expectedPlaintext, config.plaintext)
			}
		})
	}
}
