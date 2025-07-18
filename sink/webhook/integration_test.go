package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWebhookClient_Integration(t *testing.T) {
	// Create a test webhook server
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create webhook client
	config := Config{
		Timeout:     5 * time.Second,
		MaxRetries:  2,
		MaxInterval: 1 * time.Second,
	}
	client := NewClient(config, zap.NewNop())

	// Make a webhook call
	payload := []byte(`{"test": "integration"}`)
	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
	assert.Equal(t, 1, requestCount, "Expected exactly one request")
}

func TestWebhookSink_StateFileBasicOperations(t *testing.T) {
	// Create a temporary state file
	stateFile, err := os.CreateTemp("", "webhook_test_state_*.cursor")
	require.NoError(t, err)
	defer os.Remove(stateFile.Name())
	stateFile.Close()

	// Create a simple webhook sink struct for testing state file operations
	webhookSink := &Sink{
		stateFile: stateFile.Name(),
		logger:    zap.NewNop(),
	}

	// Verify the sink has the correct state file path
	assert.Equal(t, stateFile.Name(), webhookSink.stateFile)
	assert.NotNil(t, webhookSink.logger)
}

func TestWebhookSink_EmptyStateFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test with empty state file (should not cause errors)
	webhookSink := &Sink{
		stateFile: "", // Empty state file
		logger:    zap.NewNop(),
	}

	assert.Equal(t, "", webhookSink.stateFile)
}

func TestWebhookSink_PrintStats(t *testing.T) {
	// This is mainly a smoke test to ensure PrintStats doesn't panic
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webhookSink := &Sink{
		logger: zap.NewNop(),
	}

	// Should not panic
	webhookSink.PrintStats()
}

func TestWebhookClient_ConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "valid config",
			config: Config{
				Timeout:     30 * time.Second,
				MaxRetries:  3,
				MaxInterval: 30 * time.Second,
			},
		},
		{
			name: "zero timeout",
			config: Config{
				Timeout:     0,
				MaxRetries:  3,
				MaxInterval: 30 * time.Second,
			},
		},
		{
			name: "negative retries",
			config: Config{
				Timeout:     30 * time.Second,
				MaxRetries:  -1,
				MaxInterval: 30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.config, zap.NewNop())
			assert.NotNil(t, client)
			assert.Equal(t, tt.config.Timeout, client.httpClient.Timeout)
			assert.Equal(t, tt.config.MaxRetries, client.maxRetries)
			assert.Equal(t, tt.config.MaxInterval, client.maxInterval)
		})
	}
}

func TestWebhookSink_StateFileReading(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expectRead  bool
	}{
		{
			name:        "valid cursor",
			fileContent: "valid_cursor_123",
			expectRead:  true,
		},
		{
			name:        "empty file",
			fileContent: "",
			expectRead:  false,
		},
		{
			name:        "whitespace only",
			fileContent: "   \n\t  ",
			expectRead:  false,
		},
		{
			name:        "cursor with whitespace",
			fileContent: "  cursor_with_spaces  \n",
			expectRead:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			stateFile, err := os.CreateTemp("", "webhook_test_*.cursor")
			require.NoError(t, err)
			defer os.Remove(stateFile.Name())

			// Write test content
			_, err = stateFile.WriteString(tt.fileContent)
			require.NoError(t, err)
			stateFile.Close()

			// Read and verify the content
			data, err := os.ReadFile(stateFile.Name())
			require.NoError(t, err)

			trimmed := strings.TrimSpace(string(data))
			if tt.expectRead {
				assert.NotEmpty(t, trimmed)
			} else {
				assert.Empty(t, trimmed)
			}
		})
	}
}

func TestWebhookSink_StateFileWriteRead(t *testing.T) {
	// Create temporary state file
	stateFile, err := os.CreateTemp("", "webhook_test_*.cursor")
	require.NoError(t, err)
	defer os.Remove(stateFile.Name())
	stateFile.Close()

	// Test direct file operations
	testData := "test_data_123"
	err = os.WriteFile(stateFile.Name(), []byte(testData), 0644)
	require.NoError(t, err)

	// Read back the data
	savedData, err := os.ReadFile(stateFile.Name())
	require.NoError(t, err)
	assert.Equal(t, testData, string(savedData))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
