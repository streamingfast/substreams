package webhook

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type capturedRequest struct {
	header http.Header
	body   []byte
}

func captureServer(t *testing.T, status int) (*httptest.Server, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		requests = append(requests, capturedRequest{header: r.Header.Clone(), body: body})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest(nil), requests...)
	}
}

func TestClient_Call_SendsAuthHeaderAndSignature(t *testing.T) {
	server, requests := captureServer(t, http.StatusOK)

	client := NewClient(Config{
		Timeout:         time.Second,
		MaxRetries:      0,
		MaxInterval:     time.Second,
		AuthHeaderValue: "Bearer sekret",
		SigningSecret:   "whsec_test",
	}, zap.NewNop())

	payload := []byte(`{"clock":{"number":42}}`)
	require.NoError(t, client.Call(context.Background(), server.URL, payload, 42))

	got := requests()
	require.Len(t, got, 1)
	assert.Equal(t, "Bearer sekret", got[0].header.Get("Authorization"))
	assert.Equal(t, "application/json", got[0].header.Get("Content-Type"))

	signature := got[0].header.Get(SignatureHeader)
	require.NotEmpty(t, signature)
	at, err := VerifySignature([]byte("whsec_test"), got[0].body, signature)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), at, 5*time.Second)
}

func TestClient_Call_CustomAuthHeaderName(t *testing.T) {
	server, requests := captureServer(t, http.StatusOK)

	client := NewClient(Config{
		Timeout:         time.Second,
		AuthHeaderName:  "X-Api-Key",
		AuthHeaderValue: "k123",
	}, zap.NewNop())

	require.NoError(t, client.Call(context.Background(), server.URL, []byte(`{}`), 1))

	got := requests()
	require.Len(t, got, 1)
	assert.Equal(t, "k123", got[0].header.Get("X-Api-Key"))
	assert.Empty(t, got[0].header.Get("Authorization"))
}

func TestClient_Call_NoAuthByDefault(t *testing.T) {
	server, requests := captureServer(t, http.StatusOK)

	client := NewClient(Config{Timeout: time.Second}, zap.NewNop())
	require.NoError(t, client.Call(context.Background(), server.URL, []byte(`{}`), 1))

	got := requests()
	require.Len(t, got, 1)
	assert.Empty(t, got[0].header.Get("Authorization"))
	assert.Empty(t, got[0].header.Get(SignatureHeader))
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("s3cr3t")
	body := []byte(`{"a":1}`)
	at := time.Unix(1_700_000_000, 0)

	client := NewClient(Config{SigningSecret: string(secret)}, zap.NewNop())
	header := client.Sign(body, at)
	assert.Equal(t, "t=1700000000,v1=", header[:len("t=1700000000,v1=")])

	got, err := VerifySignature(secret, body, header)
	require.NoError(t, err)
	assert.Equal(t, at, got)

	_, err = VerifySignature([]byte("other"), body, header)
	assert.EqualError(t, err, "signature mismatch")

	_, err = VerifySignature(secret, []byte(`{"a":2}`), header)
	assert.EqualError(t, err, "signature mismatch")

	_, err = VerifySignature(secret, body, "v1=abc")
	assert.EqualError(t, err, "signature header needs both t and v1")

	_, err = VerifySignature(secret, body, "garbage")
	assert.EqualError(t, err, "malformed signature header")

	assert.Empty(t, NewClient(Config{}, zap.NewNop()).Sign(body, at))
}

func TestClient_Call_ReturnsDeliveryError(t *testing.T) {
	server, requests := captureServer(t, http.StatusServiceUnavailable)

	client := NewClient(Config{
		Timeout:     time.Second,
		MaxRetries:  1,
		MaxInterval: 10 * time.Millisecond,
	}, zap.NewNop())

	err := client.Call(context.Background(), server.URL, []byte(`{}`), 7)
	require.Error(t, err)

	var deliveryErr *DeliveryError
	require.ErrorAs(t, err, &deliveryErr)
	assert.Equal(t, server.URL, deliveryErr.URL)
	assert.Equal(t, uint64(7), deliveryErr.BlockNumber)
	assert.Equal(t, http.StatusServiceUnavailable, deliveryErr.StatusCode)
	assert.Equal(t, 2, deliveryErr.Attempts)
	assert.Len(t, requests(), 2)
	assert.True(t, errors.Is(err, deliveryErr.Err))
}

func TestClient_Call_ClientErrorIsNotRetried(t *testing.T) {
	server, requests := captureServer(t, http.StatusUnauthorized)

	client := NewClient(Config{Timeout: time.Second, MaxRetries: 5, MaxInterval: 10 * time.Millisecond}, zap.NewNop())

	err := client.Call(context.Background(), server.URL, []byte(`{}`), 7)
	var deliveryErr *DeliveryError
	require.ErrorAs(t, err, &deliveryErr)
	assert.Equal(t, http.StatusUnauthorized, deliveryErr.StatusCode)
	assert.Equal(t, 1, deliveryErr.Attempts)
	assert.Len(t, requests(), 1)
}
