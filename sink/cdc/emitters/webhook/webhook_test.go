package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/streamingfast/substreams/sink/cdc"
	sinkwebhook "github.com/streamingfast/substreams/sink/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func batch(block uint64, n int) *cdc.Batch {
	b := &cdc.Batch{Block: cdc.Block{Number: block, ID: "b"}, Cursor: "cur"}
	for i := range n {
		b.Events = append(b.Events, cdc.Event{ID: string(rune('a' + i)), Op: cdc.OpCreate})
	}
	return b
}

type recorder struct {
	mu       sync.Mutex
	payloads []Payload
	headers  []http.Header
	queries  []string
	bodies   [][]byte
}

func (r *recorder) handler(failFirst int) http.Handler {
	var calls atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if int(calls.Add(1)) <= failFirst {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		body, _ := io.ReadAll(req.Body)
		var p Payload
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.payloads = append(r.payloads, p)
		r.headers = append(r.headers, req.Header.Clone())
		r.queries = append(r.queries, req.URL.RawQuery)
		r.bodies = append(r.bodies, body)
		r.mu.Unlock()
	})
}

func TestEmitter_Factory_Post(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler(0))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	e, err := cdc.NewEmitter(t.Context(), "webhook://"+host+"/hook?token=xyz&header.X-Env=prod&auth=secret&auth-header=X-Auth&signing-secret=s3&max-events=2", cdc.EmitterOptions{Logger: zlog})
	require.NoError(t, err)

	require.NoError(t, e.EmitBatch(t.Context(), batch(10, 3)))
	require.NoError(t, e.EmitCompensation(t.Context(), batch(5, 1)))

	require.Len(t, rec.payloads, 3)
	assert.Equal(t, 1, rec.payloads[0].Part)
	assert.Equal(t, 2, rec.payloads[0].Parts)
	assert.Len(t, rec.payloads[0].Events, 2)
	assert.Len(t, rec.payloads[1].Events, 1)
	assert.Equal(t, uint64(10), rec.payloads[1].Block.Number)
	assert.True(t, rec.payloads[2].Compensation)
	assert.Equal(t, "cur", rec.payloads[2].Cursor)

	assert.Equal(t, "token=xyz", rec.queries[0], "unknown params are forwarded, ours are stripped")
	assert.Equal(t, "prod", rec.headers[0].Get("X-Env"))
	assert.Equal(t, "secret", rec.headers[0].Get("X-Auth"))
	_, err = sinkwebhook.VerifySignature([]byte("s3"), rec.bodies[0], rec.headers[0].Get(sinkwebhook.SignatureHeader))
	require.NoError(t, err)
}

func TestEmitter_RetriesServerErrors(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler(2))
	defer srv.Close()

	e, err := cdc.NewEmitter(t.Context(), "webhook://"+strings.TrimPrefix(srv.URL, "http://")+"/?max-interval=10ms", cdc.EmitterOptions{Logger: zlog})
	require.NoError(t, err)
	require.NoError(t, e.EmitBatch(t.Context(), batch(1, 1)))
	assert.Len(t, rec.payloads, 1)
}

func TestEmitter_ClientErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	e, err := cdc.NewEmitter(t.Context(), "webhook://"+strings.TrimPrefix(srv.URL, "http://")+"/", cdc.EmitterOptions{Logger: zlog})
	require.NoError(t, err)
	err = e.EmitBatch(t.Context(), batch(1, 1))
	require.Error(t, err)
	var derr *sinkwebhook.DeliveryError
	require.ErrorAs(t, err, &derr)
	assert.Equal(t, 403, derr.StatusCode)
}

func TestEmitter_Webhooks_UsesHTTPS(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewTLSServer(rec.handler(0))
	defer srv.Close()

	// The test server certificate is not trusted, so the call fails on TLS: that proves https is used.
	e, err := cdc.NewEmitter(t.Context(), "webhooks://"+strings.TrimPrefix(srv.URL, "https://")+"/?max-retries=0", cdc.EmitterOptions{Logger: zlog})
	require.NoError(t, err)
	require.Error(t, e.EmitBatch(t.Context(), batch(1, 1)))
	assert.Empty(t, rec.payloads)
}

func TestChunks_ByBytes(t *testing.T) {
	e, err := New(Config{URL: "http://x", MaxEvents: 100, MaxBytes: 10}, nil)
	require.NoError(t, err)
	events := []json.RawMessage{[]byte("123456"), []byte("123456"), []byte("12345678901234")}
	chunks := e.chunks(events)
	assert.Len(t, chunks, 3, "each event overflows the previous chunk, an oversize one goes alone")
}
