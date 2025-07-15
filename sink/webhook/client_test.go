package webhook

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestClient_Call_Success(t *testing.T) {
	// Create a test server that always returns 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
}

func TestClient_Call_ClientError(t *testing.T) {
	// Create a test server that returns 400 (client error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook returned client error status 400 for block 123")
}

func TestClient_Call_ServerErrorWithRetry(t *testing.T) {
	callCount := 0
	// Create a test server that fails twice then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
	assert.Equal(t, 3, callCount, "Expected 3 calls (2 failures + 1 success)")
}

func TestClient_Call_MaxRetriesExceeded(t *testing.T) {
	var callCount int32
	// Create a test server that always returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := Config{
		Timeout:     5 * time.Second,
		MaxRetries:  3,
		MaxInterval: 1 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.Error(t, err)
	finalCount := atomic.LoadInt32(&callCount)
	// Should try initial call + 3 retries = 4 total calls
	assert.Equal(t, int32(4), finalCount, "Expected 4 calls (1 initial + 3 retries)")
}

func TestClient_Call_ContextCanceled(t *testing.T) {
	// Create a test server with a delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	// Create a context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.Call(ctx, server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestClient_Call_InvalidURL(t *testing.T) {
	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), "://invalid-url", payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create webhook request")
}

func TestClient_Call_NetworkError(t *testing.T) {
	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	// Use a URL that will cause a network error
	err := client.Call(context.Background(), "http://localhost:99999", payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook call failed")
}

func TestNewClient(t *testing.T) {
	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())

	require.NotNil(t, client)
	require.NotNil(t, client.httpClient)
	assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
	assert.Equal(t, 3, client.maxRetries)
	assert.Equal(t, 30*time.Second, client.maxInterval)
}

func TestClient_Call_SuccessStatusCodes(t *testing.T) {
	testCases := []int{200, 201, 202, 204, 299}

	for _, statusCode := range testCases {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			config := Config{
				Timeout:     5 * time.Second,
				MaxRetries:  3,
				MaxInterval: 1 * time.Second,
			}
			client := NewClient(config, zap.NewNop())
			payload := []byte(`{"test": "data"}`)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := client.Call(ctx, server.URL, payload, 123)
			assert.NoError(t, err)
		})
	}
}

func TestClient_Call_RetryOnServerErrors(t *testing.T) {
	retryableStatusCodes := []int{500, 502, 503, 504}

	for _, statusCode := range retryableStatusCodes {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			var callCount int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&callCount, 1)
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			config := Config{
				Timeout:     5 * time.Second,
				MaxRetries:  3,
				MaxInterval: 1 * time.Second,
			}
			client := NewClient(config, zap.NewNop())
			payload := []byte(`{"test": "data"}`)

			err := client.Call(context.Background(), server.URL, payload, 123)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("webhook returned server error status %d for block 123", statusCode))
			finalCount := atomic.LoadInt32(&callCount)
			// Should try initial call + 3 retries = 4 total calls
			assert.Equal(t, int32(4), finalCount, "Expected 4 calls for status %d", statusCode)
		})
	}
}

func TestClient_Call_NoRetryOnClientErrors(t *testing.T) {
	clientErrorStatusCodes := []int{400, 401, 403, 404, 422}

	for _, statusCode := range clientErrorStatusCodes {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			var callCount int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&callCount, 1)
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			config := Config{
				Timeout:     5 * time.Second,
				MaxRetries:  3,
				MaxInterval: 1 * time.Second,
			}
			client := NewClient(config, zap.NewNop())
			payload := []byte(`{"test": "data"}`)

			err := client.Call(context.Background(), server.URL, payload, 123)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("webhook returned client error status %d for block 123", statusCode))
			finalCount := atomic.LoadInt32(&callCount)
			// Should only try once for client errors (no retries)
			assert.Equal(t, int32(1), finalCount, "Expected only 1 call for status %d", statusCode)
		})
	}
}

func TestClient_Call_ConfigurableParameters(t *testing.T) {
	timeout := 10 * time.Second
	maxRetries := 5
	maxInterval := 60 * time.Second

	config := Config{
		Timeout:     timeout,
		MaxRetries:  maxRetries,
		MaxInterval: maxInterval,
	}
	client := NewClient(config, zap.NewNop())

	require.NotNil(t, client)
	require.NotNil(t, client.httpClient)
	assert.Equal(t, timeout, client.httpClient.Timeout)
	assert.Equal(t, maxRetries, client.maxRetries)
	assert.Equal(t, maxInterval, client.maxInterval)
}

func TestClient_Call_CustomRetryCount(t *testing.T) {
	var callCount int32
	// Create a test server that always returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create client with custom retry count of 1
	config := Config{
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		MaxInterval: 1 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook returned server error status 500 for block 123")
	finalCount := atomic.LoadInt32(&callCount)
	// Should try initial call + 1 retry = 2 total calls
	assert.Equal(t, int32(2), finalCount, "Expected 2 calls (1 initial + 1 retry)")
}

func TestClient_Call_ZeroRetries(t *testing.T) {
	var callCount int32
	// Create a test server that always returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create client with zero retries
	config := Config{
		Timeout:     5 * time.Second,
		MaxRetries:  0,
		MaxInterval: 1 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook returned server error status 500 for block 123")
	finalCount := atomic.LoadInt32(&callCount)
	// Should only try once with zero retries
	assert.Equal(t, int32(1), finalCount, "Expected 1 call with zero retries")
}

func TestClient_Call_ContextTimeout(t *testing.T) {
	// Create a test server with a long delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.Call(ctx, server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestClient_Call_EmptyPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Empty(t, body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte{}

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
}

func TestClient_Call_LargePayload(t *testing.T) {
	expectedSize := 1024 * 1024 // 1MB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, expectedSize, len(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := make([]byte, expectedSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
}

func TestClient_Call_RequestHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		// Verify User-Agent is present (set by Go's HTTP client)
		assert.NotEmpty(t, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
}

func TestClient_Call_HTTPSEndpoint(t *testing.T) {
	// Create HTTPS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Use the test server's HTTP client that trusts the test certificates
	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	client.httpClient = server.Client()

	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
}

func TestClient_Call_RedirectResponse(t *testing.T) {
	redirectCount := 0
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount <= 2 {
			http.Redirect(w, r, finalServer.URL, http.StatusFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer redirectServer.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), redirectServer.URL, payload, 123)
	assert.NoError(t, err)
	assert.Greater(t, redirectCount, 0, "Expected at least one redirect")
}

func TestClient_Call_SlowServerWithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Use a very short timeout
	config := Config{
		Timeout:     50 * time.Millisecond,
		MaxRetries:  1,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook call failed for block 123")
}

func TestClient_Call_PayloadIntegrity(t *testing.T) {
	expectedPayload := []byte(`{"block": 123, "data": "test payload", "nested": {"key": "value"}}`)
	var receivedPayload []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedPayload = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())

	err := client.Call(context.Background(), server.URL, expectedPayload, 123)
	assert.NoError(t, err)
	assert.Equal(t, expectedPayload, receivedPayload, "Payload should be transmitted without modification")
}

func TestClient_Call_MultipleRequestsSamePayload(t *testing.T) {
	var callCount int32
	expectedPayload := []byte(`{"consistent": "data"}`)
	var payloads [][]byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		mu.Lock()
		payloads = append(payloads, body)
		mu.Unlock()

		// Fail first two calls, succeed on third
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := Config{
		Timeout:     5 * time.Second,
		MaxRetries:  3,
		MaxInterval: 1 * time.Second,
	}
	client := NewClient(config, zap.NewNop())

	err := client.Call(context.Background(), server.URL, expectedPayload, 123)
	assert.NoError(t, err)
	finalCount := atomic.LoadInt32(&callCount)
	assert.Equal(t, int32(3), finalCount, "Expected 3 calls")

	// Verify all retries sent the same payload
	mu.Lock()
	defer mu.Unlock()
	for i, payload := range payloads {
		assert.Equal(t, expectedPayload, payload, "Payload should be identical on retry %d", i+1)
	}
}

func BenchmarkClient_Call_Success(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "data"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := client.Call(context.Background(), server.URL, payload, 123)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_Call_LargePayload(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		MaxInterval: 30 * time.Second,
	}
	client := NewClient(config, zap.NewNop())
	payload := make([]byte, 1024*1024) // 1MB payload

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := client.Call(context.Background(), server.URL, payload, 123)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestClient_Call_InfiniteRetries(t *testing.T) {
	// Test infinite retries (-1) with eventual success
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount < 5 {
			// Fail first 4 requests
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			// Succeed on 5th request
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := Config{
		Timeout:     5 * time.Second,
		MaxRetries:  -1,                     // Infinite retries
		MaxInterval: 100 * time.Millisecond, // Short interval for faster test
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "infinite retries"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.NoError(t, err)
	assert.Equal(t, 5, requestCount, "Should have made 5 requests (4 failures + 1 success)")
}

func TestClient_Call_InfiniteRetriesWithContextCancellation(t *testing.T) {
	// Test that infinite retries respect context cancellation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return server error to force retries
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := Config{
		Timeout:     1 * time.Second,
		MaxRetries:  -1, // Infinite retries
		MaxInterval: 50 * time.Millisecond,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "context cancel"}`)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := client.Call(ctx, server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestClient_Call_InfiniteRetriesClientError(t *testing.T) {
	// Test that infinite retries still respect permanent errors (4xx)
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusBadRequest) // 400 - permanent error
	}))
	defer server.Close()

	config := Config{
		Timeout:     1 * time.Second,
		MaxRetries:  -1, // Infinite retries
		MaxInterval: 10 * time.Millisecond,
	}
	client := NewClient(config, zap.NewNop())
	payload := []byte(`{"test": "client error"}`)

	err := client.Call(context.Background(), server.URL, payload, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client error status 400")
	assert.Equal(t, 1, requestCount, "Should only make one request for permanent error")
}

func TestClient_Call_ConfigValidation(t *testing.T) {
	// Test that different MaxRetries values work correctly
	testCases := []struct {
		name       string
		maxRetries int
		expected   string
	}{
		{
			name:       "zero retries",
			maxRetries: 0,
			expected:   "no retries",
		},
		{
			name:       "limited retries",
			maxRetries: 3,
			expected:   "limited retries",
		},
		{
			name:       "infinite retries",
			maxRetries: -1,
			expected:   "infinite retries",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				Timeout:     30 * time.Second,
				MaxRetries:  tc.maxRetries,
				MaxInterval: 30 * time.Second,
			}
			client := NewClient(config, zap.NewNop())
			assert.Equal(t, tc.maxRetries, client.maxRetries)
		})
	}
}
