package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/streamingfast/dregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFoundationalStoreTimeout(t *testing.T) {
	orig := foundationalStoreResolveTimeout
	foundationalStoreResolveTimeout = 50 * time.Millisecond
	t.Cleanup(func() { foundationalStoreResolveTimeout = orig })

	start := time.Now()
	_, err := resolveFoundationalStore(t.Context(), hangResolver{}, "slow-store")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}

func TestResolveFoundationalStoreSucceedsWithinTimeout(t *testing.T) {
	got, err := resolveFoundationalStore(t.Context(), staticFoundationalResolver{"known": "stores.example.com:9000"}, "known")
	require.NoError(t, err)
	assert.Equal(t, "stores.example.com:9000", got.Address)
}

type hangResolver struct{}

func (hangResolver) Resolve(ctx context.Context, _ string) (*dregistry.Endpoint, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type staticFoundationalResolver map[string]string

func (r staticFoundationalResolver) Resolve(_ context.Context, identifier string) (*dregistry.Endpoint, error) {
	addr, ok := r[identifier]
	if !ok {
		return nil, errors.New("not found")
	}
	return &dregistry.Endpoint{Address: addr}, nil
}
