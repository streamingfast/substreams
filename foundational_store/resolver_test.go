package foundational_store

import (
	"context"
	"fmt"
	"testing"

	"github.com/streamingfast/dregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResolverJSONThenPassthrough(t *testing.T) {
	resolver, err := NewResolver(map[string]string{
		"known": "stores.example.com:9000",
	}, "", nil)
	require.NoError(t, err)

	got, err := resolver.Resolve(t.Context(), "known")
	require.NoError(t, err)
	assert.Equal(t, "stores.example.com:9000", got.Address)

	got, err = resolver.Resolve(t.Context(), "unknown.example.com:9000")
	require.NoError(t, err)
	assert.Equal(t, "unknown.example.com:9000", got.Address)
}

func TestNewPassthroughResolver(t *testing.T) {
	resolver, err := NewPassthroughResolver()
	require.NoError(t, err)

	got, err := resolver.Resolve(t.Context(), "direct.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, "direct.example.com:443", got.Address)
	assert.True(t, got.TLS)
}

func TestLookup(t *testing.T) {
	got, err := Lookup(map[string]string{"known": "grpc://stores.example.com:9000"}, "known")
	require.NoError(t, err)
	assert.Equal(t, "stores.example.com:9000", got.Address)
	assert.False(t, got.TLS)

	_, err = Lookup(map[string]string{"known": "grpc://stores.example.com:9000"}, "missing")
	require.Error(t, err)
	assert.True(t, dregistry.IsNotFound(err))
}

func TestDeploymentID(t *testing.T) {
	assert.Equal(t, "deadbeef", DeploymentID("deadbeef@v1.2.3"))
	assert.Equal(t, "deadbeef", DeploymentID("deadbeef"))
	assert.Equal(t, "acme/token-balances", DeploymentID("acme/token-balances@v0.1.0"))
	assert.Equal(t, "acme/token-balances", DeploymentID("acme/token-balances"))
	assert.Equal(t, "dead@vbeef", DeploymentID("dead@vbeef@v2"))
	assert.Equal(t, "@v1.0.0", DeploymentID("@v1.0.0"))
	assert.Equal(t, "", DeploymentID(""))
}

func TestStripDeploymentID(t *testing.T) {
	var gotID string
	inner := resolverFunc(func(id string) (*dregistry.Endpoint, error) {
		gotID = id
		return &dregistry.Endpoint{Address: id}, nil
	})

	wrapped := stripDeploymentID{inner: inner}
	got, err := wrapped.Resolve(t.Context(), "deadbeef@v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef", gotID)
	assert.Equal(t, "deadbeef", got.Address)

	got, err = wrapped.Resolve(t.Context(), "acme/token-balances@v0.1.0")
	require.NoError(t, err)
	assert.Equal(t, "acme/token-balances", gotID)
	assert.Equal(t, "acme/token-balances", got.Address)
}

func TestStripDeploymentIDOrgReferenceNotFoundStopsChain(t *testing.T) {
	notFound := resolverFunc(func(id string) (*dregistry.Endpoint, error) {
		return nil, fmt.Errorf("%w: %q", dregistry.ErrNotFound, id)
	})
	passthrough, err := NewPassthroughResolver()
	require.NoError(t, err)
	chain := dregistry.Chain(stripDeploymentID{inner: notFound}, passthrough)

	_, err = chain.Resolve(t.Context(), "acme/token-balances@v0.1.0")
	require.Error(t, err)
	assert.False(t, dregistry.IsNotFound(err))
	assert.Contains(t, err.Error(), `no live hosted store matches "acme/token-balances"`)

	got, err := chain.Resolve(t.Context(), "grpc://stores.example.com:9000")
	require.NoError(t, err)
	assert.Equal(t, "stores.example.com:9000", got.Address)

	got, err = chain.Resolve(t.Context(), "deadbeef")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef", got.Address)
}

func TestControlPlaneDSN(t *testing.T) {
	assert.Equal(t, "controlplane://registry.example.com:9000?insecure=true", controlPlaneDSN("registry.example.com:9000"))
	assert.Equal(t, "controlplane://registry.example.com:9000", controlPlaneDSN("controlplane://registry.example.com:9000"))
	assert.Equal(t, "controlplane://registry.example.com:9000?insecure=true", controlPlaneDSN("grpc://registry.example.com:9000"))
	assert.Equal(t, "controlplane://registry.example.com:443?insecure=false", controlPlaneDSN("grpcs://registry.example.com:443"))
}

func TestNewStaticResolver(t *testing.T) {
	resolver, err := NewStaticResolver(map[string]string{
		"known": "grpc://stores.example.com:9000",
	})
	require.NoError(t, err)

	got, err := resolver.Resolve(t.Context(), "known")
	require.NoError(t, err)
	assert.Equal(t, "stores.example.com:9000", got.Address)
	assert.False(t, got.TLS)

	_, err = resolver.Resolve(t.Context(), "missing")
	require.Error(t, err)
	assert.True(t, dregistry.IsNotFound(err))

	empty, err := NewStaticResolver(nil)
	require.NoError(t, err)
	_, err = empty.Resolve(t.Context(), "anything")
	require.Error(t, err)
	assert.True(t, dregistry.IsNotFound(err))
}

func TestEncodeEndpoint(t *testing.T) {
	assert.Equal(t, "grpc://stores.example.com:9000", EncodeEndpoint(&dregistry.Endpoint{Address: "stores.example.com:9000"}))
	assert.Equal(t, "grpcs://stores.example.com:443", EncodeEndpoint(&dregistry.Endpoint{Address: "stores.example.com:443", TLS: true}))
	assert.Equal(t, "", EncodeEndpoint(nil))
}

func TestResolveIdentifiers(t *testing.T) {
	resolver, err := NewResolver(map[string]string{
		"known": "stores.example.com:9000",
	}, "", nil)
	require.NoError(t, err)

	got, err := ResolveIdentifiers(t.Context(), resolver, []string{"known", "other.example.com:443"})
	require.NoError(t, err)
	assert.Equal(t, "grpc://stores.example.com:9000", got["known"])
	assert.Equal(t, "grpcs://other.example.com:443", got["other.example.com:443"])
}

type resolverFunc func(string) (*dregistry.Endpoint, error)

func (f resolverFunc) Resolve(_ context.Context, identifier string) (*dregistry.Endpoint, error) {
	return f(identifier)
}
