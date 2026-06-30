package wasm

import (
	"context"
	"errors"
	"testing"

	pbmodel "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/model/v2"
	pbservice "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/service/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// step is one scripted response from the fake foundational store.
type step struct {
	resp *pbservice.GetResponse
	err  error
}

// fakeStoreClient returns the scripted steps in order, repeating the last one
// once exhausted (so "always Unavailable" scripts can be a single step).
type fakeStoreClient struct {
	steps []step
	calls int
}

func (f *fakeStoreClient) next() (*pbservice.GetResponse, error) {
	i := f.calls
	f.calls++
	if i >= len(f.steps) {
		i = len(f.steps) - 1
	}
	return f.steps[i].resp, f.steps[i].err
}

func (f *fakeStoreClient) Get(ctx context.Context, in *pbservice.GetRequest, opts ...grpc.CallOption) (*pbservice.GetResponse, error) {
	return f.next()
}

func (f *fakeStoreClient) GetFirst(ctx context.Context, in *pbservice.GetRequest, opts ...grpc.CallOption) (*pbservice.GetResponse, error) {
	return f.next()
}

// runFoundationalGet drives DoFoundationalStoreGet / DoFoundationalStoreGetFirst
// the same way pipeline.execute() does: any panic from the wasm host function is
// recovered into an error. That recovered error is exactly what
// process_block.go inspects with errors.Is to decide whether to cache it.
func runFoundationalGet(client pbservice.StoreClient, useFirst bool) (entries *pbmodel.QueriedEntries, recovered error) {
	call := NewCall(
		context.Background(),
		&pbsubstreams.Clock{Number: 100},
		"test_module",
		"entrypoint",
		nil,
		nil,
		false,
		[]pbservice.StoreClient{client},
	)

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				recovered = err
			} else {
				recovered = errors.New("non-error panic")
			}
		}
	}()

	keys := &pbmodel.Keys{Keys: []*pbmodel.Key{{Bytes: []byte("some-key")}}}
	if useFirst {
		entries = call.DoFoundationalStoreGetFirst(0, keys)
	} else {
		entries = call.DoFoundationalStoreGet(0, keys)
	}
	return
}

// assertFatalNonDeterministic asserts the recovered error bubbles up to the user
// uncached: it must be a foundational-store fatal error and must NOT be a
// deterministic error (which would be written to the cache by process_block.go).
func assertFatalNonDeterministic(t *testing.T, recovered error) {
	t.Helper()
	require.Error(t, recovered)
	assert.True(t, errors.Is(recovered, ErrFoundationalStoreFatal), "must be a foundational store fatal error, got: %v", recovered)
	assert.False(t, errors.Is(recovered, ErrWasmDeterministicExec), "must NOT be deterministic (would be cached), got: %v", recovered)
	assert.False(t, errors.Is(recovered, ErrFoundationalStoreCanceled), "must not be the upstream-cancel error, got: %v", recovered)
}

func TestFoundationalStore_FatalErrorsBubbleUp(t *testing.T) {
	// auth failures and org id mismatches must fail fast instead of retrying
	// until the global deadline.
	cases := []struct {
		name string
		err  error
	}{
		{"unauthenticated", status.Error(codes.Unauthenticated, "missing authenticated context")},
		{"permission_denied", status.Error(codes.PermissionDenied, "organization id mismatch")},
	}

	for _, c := range cases {
		t.Run(c.name+"/get", func(t *testing.T) {
			client := &fakeStoreClient{steps: []step{{err: c.err}}}
			entries, recovered := runFoundationalGet(client, false)
			assert.Nil(t, entries)
			assertFatalNonDeterministic(t, recovered)
			assert.Equal(t, 1, client.calls, "fatal error must not be retried")
		})
		t.Run(c.name+"/get_first", func(t *testing.T) {
			client := &fakeStoreClient{steps: []step{{err: c.err}}}
			entries, recovered := runFoundationalGet(client, true)
			assert.Nil(t, entries)
			assertFatalNonDeterministic(t, recovered)
			assert.Equal(t, 1, client.calls, "fatal error must not be retried")
		})
	}
}

func TestFoundationalStore_UnreachableRetriesThenBubblesUp(t *testing.T) {
	prev := foundationalStoreMaxUnreachableRetries
	foundationalStoreMaxUnreachableRetries = 2
	defer func() { foundationalStoreMaxUnreachableRetries = prev }()

	// Unavailable (unreachable) is retried up to the cap, then bubbles up as a
	// fatal non-deterministic error.
	client := &fakeStoreClient{steps: []step{{err: status.Error(codes.Unavailable, "connection refused")}}}
	entries, recovered := runFoundationalGet(client, false)
	assert.Nil(t, entries)
	assertFatalNonDeterministic(t, recovered)
	assert.Greater(t, client.calls, foundationalStoreMaxUnreachableRetries, "should have retried before giving up")
}

func TestFoundationalStore_TransientUnavailableIsAbsorbed(t *testing.T) {
	// A single Unavailable blip followed by success must NOT bubble up: the call
	// retries and returns the entries.
	wantEntries := &pbmodel.QueriedEntries{}
	client := &fakeStoreClient{steps: []step{
		{err: status.Error(codes.Unavailable, "rolling restart")},
		{resp: &pbservice.GetResponse{BlockReached: true, Entries: wantEntries}},
	}}

	entries, recovered := runFoundationalGet(client, false)
	require.NoError(t, recovered)
	assert.Same(t, wantEntries, entries)
	assert.Equal(t, 2, client.calls)
}

func TestFoundationalStore_BlockNotReachedRetries(t *testing.T) {
	// A non-error response that hasn't reached the block yet keeps retrying
	// (store catching up) and is not treated as fatal.
	wantEntries := &pbmodel.QueriedEntries{}
	client := &fakeStoreClient{steps: []step{
		{resp: &pbservice.GetResponse{BlockReached: false}},
		{resp: &pbservice.GetResponse{BlockReached: false}},
		{resp: &pbservice.GetResponse{BlockReached: true, Entries: wantEntries}},
	}}

	entries, recovered := runFoundationalGet(client, false)
	require.NoError(t, recovered)
	assert.Same(t, wantEntries, entries)
	assert.Equal(t, 3, client.calls)
}
