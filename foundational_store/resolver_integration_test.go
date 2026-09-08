package foundational_store

import (
	"context"
	"net"
	"testing"

	"github.com/streamingfast/dregistry"
	pbregistry "github.com/streamingfast/dregistry/pb/sf/registry/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// Locks the 3-tier lookup that existed on develop (JSON map → control-plane
// GetFoundationStore → identifier as endpoint), including the @v strip that
// only applies to the control-plane hop.
func TestResolver_ThreeTierMatchesDevelopBehavior(t *testing.T) {
	addr := startFakeRegistry(t, map[string]*pbregistry.FoundationStoreEntry{
		"deadbeef": {
			DeploymentId:     "deadbeef",
			Endpoint:         "public.example.com:443",
			Tls:              true,
			InternalEndpoint: "internal.example.com:9000",
			InternalTls:      false,
			AuthRequired:     true,
		},
	})

	resolver, err := NewResolver(map[string]string{
		"legacy-store": "json.example.com:9000",
	}, addr, nil)
	require.NoError(t, err)

	got, err := resolver.Resolve(t.Context(), "legacy-store")
	require.NoError(t, err)
	assert.Equal(t, "json.example.com:9000", got.Address)

	// Control-plane hit prefers the internal endpoint and copies proto TLS/auth.
	got, err = resolver.Resolve(t.Context(), "deadbeef")
	require.NoError(t, err)
	assert.Equal(t, &dregistry.Endpoint{
		Address:      "internal.example.com:9000",
		AuthRequired: true,
	}, got)

	// @v suffix is stripped only for the control-plane hop.
	got, err = resolver.Resolve(t.Context(), "deadbeef@v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "internal.example.com:9000", got.Address)

	// Miss on JSON and control-plane falls through to identifier-as-endpoint.
	got, err = resolver.Resolve(t.Context(), "direct.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, &dregistry.Endpoint{
		Address: "direct.example.com:443",
		TLS:     true,
	}, got)
}

type fakeRegistry struct {
	pbregistry.UnimplementedFoundationStoreRegistryServiceServer
	entries map[string]*pbregistry.FoundationStoreEntry
}

func (f *fakeRegistry) GetFoundationStore(_ context.Context, req *pbregistry.GetFoundationStoreRequest) (*pbregistry.GetFoundationStoreResponse, error) {
	entry, ok := f.entries[req.DeploymentId]
	if !ok {
		return &pbregistry.GetFoundationStoreResponse{Found: false}, nil
	}
	return &pbregistry.GetFoundationStoreResponse{Found: true, Entry: entry}, nil
}

func startFakeRegistry(t *testing.T, entries map[string]*pbregistry.FoundationStoreEntry) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	pbregistry.RegisterFoundationStoreRegistryServiceServer(server, &fakeRegistry{entries: entries})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String()
}
