package webhook

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
)

// Client holds the HTTP client and retry configuration for webhook calls.
// It implements exponential backoff retry logic for transient failures while
// avoiding retries for permanent client errors (4xx status codes).
type Client struct {
	httpClient  *http.Client  // HTTP client with configured timeout
	maxRetries  int           // Maximum number of retry attempts
	maxInterval time.Duration // Maximum interval between retries
	logger      *zap.Logger   // Logger for webhook operations
}

// Config holds configuration for the webhook client
type Config struct {
	Timeout     time.Duration // HTTP request timeout for individual calls
	MaxRetries  int           // Maximum number of retry attempts for transient failures (-1 for infinite retries)
	MaxInterval time.Duration // Maximum interval between retries (exponential backoff cap)
}

// NewClient creates a new webhook client with retry configuration.
func NewClient(config Config, logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		maxRetries:  config.MaxRetries,
		maxInterval: config.MaxInterval,
		logger:      logger,
	}
}

// Call makes a webhook call with exponential backoff retry logic.
// It retries on network errors and server errors (5xx), but not on client errors (4xx).
// The function respects context cancellation and implements proper error classification.
func (c *Client) Call(ctx context.Context, url string, payload []byte, blockNumber uint64) error {
	operation := func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
		if err != nil {
			// Request creation errors are permanent (bad URL, etc.)
			return backoff.Permanent(fmt.Errorf("failed to create webhook request for block %d: %w", blockNumber, err))
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Network errors are typically transient and should be retried
			return fmt.Errorf("webhook call failed for block %d: %w", blockNumber, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Client errors (4xx) are permanent - bad request, auth, etc.
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return backoff.Permanent(fmt.Errorf("webhook returned client error status %d for block %d", resp.StatusCode, blockNumber))
			}
			// Server errors (5xx) are transient and should be retried
			return fmt.Errorf("webhook returned server error status %d for block %d", resp.StatusCode, blockNumber)
		}

		return nil
	}

	b := backoff.NewExponentialBackOff()
	b.MaxInterval = c.maxInterval
	b.MaxElapsedTime = 0 // No overall timeout, let context handle it

	var retryBackoff backoff.BackOff
	if c.maxRetries == -1 {
		// Infinite retries - no max retry limit
		retryBackoff = b
	} else {
		// Limited retries
		retryBackoff = backoff.WithMaxRetries(b, uint64(c.maxRetries))
	}
	retryBackoff = backoff.WithContext(retryBackoff, ctx)

	err := backoff.Retry(operation, retryBackoff)
	if err != nil {
		c.logger.Warn("webhook call failed after all retries",
			zap.Error(err),
			zap.Uint64("block", blockNumber),
			zap.String("url", url))
		return err
	}

	c.logger.Info("webhook call successful",
		zap.Uint64("block", blockNumber),
		zap.String("url", url))
	return nil
}
