package tests_e2e

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tools/devenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"google.golang.org/grpc"

	pbregistry "github.com/streamingfast/dregistry/pb/sf/registry/v1"
)

// The dummy wasm does not take a foundational-store argument, so after a
// successful resolve the request dies in the wasm ABI ("too many arguments").
// That is still the proof we want: the identifier was resolved and the
// pipeline started. A resolve failure never gets that far.

func TestFoundationalStoreJSONResolution(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 200)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	configPath := filepath.Join(tmpDir, "foundational-stores.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"e2e-store":"127.0.0.1:1"}`), 0o644))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, endpoint := startTier1(t, ctx, devenv.Tier1Config{
		TmpDir:                       tmpDir,
		RelayerEndpoint:              relayerEndpoint(t, ctx, container),
		Tier2Endpoint:                t2Endpoint,
		MaxWorkersPerSession:         5,
		MetricsPrefix:                "test-fstore-json",
		FoundationalStoresConfigPath: configPath,
	}, zlog)
	defer shutdownStack(app, app2)

	session, err := runFoundationalStoreRequest(t, endpoint)
	require.NotNil(t, session, "resolution happens before the session; a missing session means the identifier never resolved")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many arguments")
}

func TestFoundationalStoreControlPlaneResolution(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 200)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	registryAddr := startFakeRegistry(t, map[string]*pbregistry.FoundationStoreEntry{
		"e2e-store": {
			DeploymentId: "e2e-store",
			Endpoint:     "127.0.0.1:1",
		},
	})

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, endpoint := startTier1(t, ctx, devenv.Tier1Config{
		TmpDir:                     tmpDir,
		RelayerEndpoint:            relayerEndpoint(t, ctx, container),
		Tier2Endpoint:              t2Endpoint,
		MaxWorkersPerSession:       5,
		MetricsPrefix:              "test-fstore-cp",
		HostedStoreRegistryAddress: registryAddr,
	}, zlog)
	defer shutdownStack(app, app2)

	session, err := runFoundationalStoreRequest(t, endpoint)
	require.NotNil(t, session, "resolution happens before the session; a missing session means the identifier never resolved")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many arguments")
}

func TestFoundationalStoreControlPlaneUnreachable(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 200)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, endpoint := startTier1(t, ctx, devenv.Tier1Config{
		TmpDir:                     tmpDir,
		RelayerEndpoint:            relayerEndpoint(t, ctx, container),
		Tier2Endpoint:              t2Endpoint,
		MaxWorkersPerSession:       5,
		MetricsPrefix:              "test-fstore-down",
		HostedStoreRegistryAddress: "127.0.0.1:1",
	}, zlog)
	defer shutdownStack(app, app2)

	session, err := runFoundationalStoreRequest(t, endpoint)
	require.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "foundational store")
}

func shutdownStack(tier1 interface {
	Shutdown(error)
	Terminated() <-chan struct{}
}, tier2 interface {
	Shutdown(error)
	Terminated() <-chan struct{}
}) {
	tier1.Shutdown(nil)
	tier2.Shutdown(nil)
	<-tier1.Terminated()
	<-tier2.Terminated()
}

func runFoundationalStoreRequest(t *testing.T, endpoint string) (*pbsubstreamsrpcv2.SessionInit, error) {
	t.Helper()

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.2.0.spkg").Read()
	require.NoError(t, err)
	addFoundationalStoreInput(pkg.Package, "map_events", "e2e-store")

	_, session, err := RunRequest(t, &pbsubstreamsrpcv3.Request{
		StartBlockNum:   100,
		StopBlockNum:    105,
		FinalBlocksOnly: true,
		OutputModule:    "map_events",
		Package:         pkg.Package,
	}, endpoint)
	if err == io.EOF {
		err = nil
	}
	return session, err
}

func addFoundationalStoreInput(pkg *pbsubstreams.Package, module, identifier string) {
	if pkg.Modules == nil {
		return
	}
	for _, m := range pkg.Modules.Modules {
		if m.Name != module {
			continue
		}
		m.Inputs = append(m.Inputs, &pbsubstreams.Module_Input{
			Input: &pbsubstreams.Module_Input_FoundationalStore{
				FoundationalStore: &pbsubstreams.Module_FoundationalStore{Identifier: identifier},
			},
		})
	}
}

type e2eFakeRegistry struct {
	pbregistry.UnimplementedFoundationStoreRegistryServiceServer
	entries map[string]*pbregistry.FoundationStoreEntry
}

func (f *e2eFakeRegistry) GetFoundationStore(_ context.Context, req *pbregistry.GetFoundationStoreRequest) (*pbregistry.GetFoundationStoreResponse, error) {
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
	pbregistry.RegisterFoundationStoreRegistryServiceServer(server, &e2eFakeRegistry{entries: entries})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String()
}
