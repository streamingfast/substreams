package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
)

// SignatureHeader carries the HMAC signature of the request body when a
// signing secret is configured. Its value has the form
// "t=<unix seconds>,v1=<hex>", where <hex> is HMAC-SHA256 over the string
// "<t>.<body>" keyed with the signing secret. Receivers recompute the HMAC
// over the raw body bytes they read and compare with a constant-time
// comparison; the timestamp lets them reject replays older than they accept.
const SignatureHeader = "X-Substreams-Signature"

// DefaultAuthHeaderName is the header the auth value is sent under when no
// name is configured.
const DefaultAuthHeaderName = "Authorization"

// Client holds the HTTP client and retry configuration for webhook calls.
// It implements exponential backoff retry logic for transient failures while
// avoiding retries for permanent client errors (4xx status codes).
type Client struct {
	httpClient      *http.Client  // HTTP client with configured timeout
	maxRetries      int           // Maximum number of retry attempts
	maxInterval     time.Duration // Maximum interval between retries
	authHeaderName  string
	authHeaderValue string
	signingSecret   []byte
	logger          *zap.Logger // Logger for webhook operations
}

// Config holds configuration for the webhook client
type Config struct {
	Timeout     time.Duration // HTTP request timeout for individual calls
	MaxRetries  int           // Maximum number of retry attempts for transient failures (-1 for infinite retries)
	MaxInterval time.Duration // Maximum interval between retries (exponential backoff cap)

	// AuthHeaderName and AuthHeaderValue are sent verbatim on every request
	// when AuthHeaderValue is not empty. AuthHeaderName defaults to
	// DefaultAuthHeaderName.
	AuthHeaderName  string
	AuthHeaderValue string

	// SigningSecret, when not empty, adds a SignatureHeader to every request.
	SigningSecret string
}

// DeliveryError is returned by Client.Call once every attempt for a payload
// has failed. It carries what a receiver's operator needs to see: the last
// HTTP status (0 when no response was received), how many attempts were made,
// and the last underlying error.
type DeliveryError struct {
	URL         string
	BlockNumber uint64
	StatusCode  int
	Attempts    int
	Err         error
}

func (e *DeliveryError) Error() string {
	return fmt.Sprintf("webhook delivery of block %d to %s failed after %d attempt(s): %v", e.BlockNumber, e.URL, e.Attempts, e.Err)
}

func (e *DeliveryError) Unwrap() error { return e.Err }

// NewClient creates a new webhook client with retry configuration.
func NewClient(config Config, logger *zap.Logger) *Client {
	authHeaderName := config.AuthHeaderName
	if authHeaderName == "" {
		authHeaderName = DefaultAuthHeaderName
	}

	var signingSecret []byte
	if config.SigningSecret != "" {
		signingSecret = []byte(config.SigningSecret)
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		maxRetries:      config.MaxRetries,
		maxInterval:     config.MaxInterval,
		authHeaderName:  authHeaderName,
		authHeaderValue: config.AuthHeaderValue,
		signingSecret:   signingSecret,
		logger:          logger,
	}
}

// Sign computes the SignatureHeader value for body at the given time. It
// returns an empty string when no signing secret is configured.
func (c *Client) Sign(body []byte, at time.Time) string {
	if len(c.signingSecret) == 0 {
		return ""
	}
	return "t=" + strconv.FormatInt(at.Unix(), 10) + ",v1=" + signPayload(c.signingSecret, body, at.Unix())
}

func signPayload(secret []byte, body []byte, timestamp int64) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks a SignatureHeader value against body using secret.
// It returns the timestamp embedded in the header so callers can apply their
// own tolerance window. Exposed for receivers written in Go and for tests.
func VerifySignature(secret []byte, body []byte, header string) (time.Time, error) {
	var timestamp int64
	var haveTimestamp bool
	var v1 string
	for field := range strings.SplitSeq(header, ",") {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return time.Time{}, errors.New("malformed signature header")
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("malformed signature timestamp: %w", err)
			}
			timestamp, haveTimestamp = parsed, true
		case "v1":
			v1 = value
		}
	}
	if !haveTimestamp || v1 == "" {
		return time.Time{}, errors.New("signature header needs both t and v1")
	}

	if !hmac.Equal([]byte(signPayload(secret, body, timestamp)), []byte(v1)) {
		return time.Time{}, errors.New("signature mismatch")
	}
	return time.Unix(timestamp, 0), nil
}

// Call makes a webhook call with exponential backoff retry logic.
// It retries on network errors and server errors (5xx), but not on client errors (4xx).
// The function respects context cancellation and implements proper error classification.
// When every attempt fails the returned error is a *DeliveryError.
func (c *Client) Call(ctx context.Context, url string, payload []byte, blockNumber uint64) error {
	attempts := 0
	lastStatus := 0

	operation := func() error {
		attempts++
		lastStatus = 0

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
		if err != nil {
			// Request creation errors are permanent (bad URL, etc.)
			return backoff.Permanent(fmt.Errorf("failed to create webhook request for block %d: %w", blockNumber, err))
		}

		req.Header.Set("Content-Type", "application/json")
		if c.authHeaderValue != "" {
			req.Header.Set(c.authHeaderName, c.authHeaderValue)
		}
		if signature := c.Sign(payload, time.Now()); signature != "" {
			req.Header.Set(SignatureHeader, signature)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Network errors are typically transient and should be retried
			return fmt.Errorf("webhook call failed for block %d: %w", blockNumber, err)
		}
		defer resp.Body.Close()

		lastStatus = resp.StatusCode
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
			zap.Int("attempts", attempts),
			zap.Int("last_status", lastStatus),
			zap.String("url", url))
		return &DeliveryError{URL: url, BlockNumber: blockNumber, StatusCode: lastStatus, Attempts: attempts, Err: err}
	}

	c.logger.Info("webhook call successful",
		zap.Uint64("block", blockNumber),
		zap.String("url", url))
	return nil
}
