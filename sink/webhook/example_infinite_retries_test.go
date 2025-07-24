package webhook

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestInfiniteRetryExamples demonstrates how infinite retries work with the webhook client.
// This comprehensive test shows different real-world usage patterns.
func TestInfiniteRetryExamples(t *testing.T) {
	t.Run("basic infinite retries", func(t *testing.T) {
		// Counter to track server behavior
		var requestCount int64

		// Create a server that fails initially but eventually succeeds
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&requestCount, 1)

			// Simulate different failure scenarios
			switch {
			case count <= 2:
				// First 2 requests: network-level failures (5xx errors)
				w.WriteHeader(http.StatusServiceUnavailable)
			case count == 3:
				// Third request: temporary server error
				w.WriteHeader(http.StatusBadGateway)
			case count >= 4:
				// Fourth request onwards: success
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "received"}`))
			}
		}))
		defer server.Close()

		// Create webhook client with infinite retries
		config := Config{
			Timeout:     2 * time.Second,
			MaxRetries:  -1,                     // Infinite retries
			MaxInterval: 200 * time.Millisecond, // Fast retries for demo
		}

		client := NewClient(config, zap.NewNop())
		payload := []byte(`{"module": "example_module", "block": 12345}`)

		// Make the webhook call - this will retry until it succeeds
		err := client.Call(context.Background(), server.URL, payload, 12345)

		// Should eventually succeed
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, atomic.LoadInt64(&requestCount), int64(4))
	})

	t.Run("infinite retries with timeout", func(t *testing.T) {
		var requestCount int64

		// Create a server that always fails
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&requestCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		// Create webhook client with infinite retries
		config := Config{
			Timeout:     1 * time.Second,
			MaxRetries:  -1,                     // Infinite retries
			MaxInterval: 100 * time.Millisecond, // Fast retries for demo
		}

		client := NewClient(config, zap.NewNop())
		payload := []byte(`{"test": "timeout example"}`)

		// Create context with timeout to limit how long we retry
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()

		err := client.Call(ctx, server.URL, payload, 12345)

		// Should fail due to context timeout, not retry exhaustion
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")

		// Should have made multiple requests but stopped due to context
		assert.GreaterOrEqual(t, atomic.LoadInt64(&requestCount), int64(1))
	})

	t.Run("production pattern", func(t *testing.T) {
		var (
			requestCount int64
			successCount int64
			failureCount int64
		)

		// Simulate a production webhook endpoint with occasional failures
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&requestCount, 1)

			// Simulate 20% failure rate
			if count%5 == 0 {
				atomic.AddInt64(&failureCount, 1)
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			atomic.AddInt64(&successCount, 1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Production configuration for critical webhooks
		config := Config{
			Timeout:     30 * time.Second, // Generous timeout
			MaxRetries:  -1,               // Infinite retries for critical data
			MaxInterval: 60 * time.Second, // Cap backoff at 1 minute
		}

		logger, _ := zap.NewDevelopment()
		client := NewClient(config, logger)

		// Simulate processing multiple blocks
		blocks := []uint64{12345, 12346, 12347, 12348, 12349}

		for _, blockNum := range blocks {
			payload := []byte(fmt.Sprintf(`{
				"module": "critical_events",
				"block": %d,
				"timestamp": "2023-12-15T10:30:45Z",
				"type": "type.googleapis.com/critical.Event",
				"payload": {
					"block_number": %d,
					"critical_data": "must_be_delivered"
				}
			}`, blockNum, blockNum))

			// In production, you might want to add per-block timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

			err := client.Call(ctx, server.URL, payload, blockNum)
			cancel()

			// With infinite retries, this should eventually succeed
			assert.NoError(t, err)

			// Brief pause between blocks (not needed in real usage)
			time.Sleep(10 * time.Millisecond)
		}

		// All blocks should eventually succeed with infinite retries
		assert.Equal(t, int64(len(blocks)), atomic.LoadInt64(&successCount))
	})
}
