package manifest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOrgPackageReference(t *testing.T) {
	tests := []struct {
		input          string
		org            string
		packageName    string
		version        string
		isOrgReference bool
		expectError    string
	}{
		{input: "sf/uniswap-v4@v1.0.0", org: "sf", packageName: "uniswap-v4", version: "v1.0.0", isOrgReference: true},
		{input: "my-org/my_pkg@v0.1.0-rc.1", org: "my-org", packageName: "my_pkg", version: "v0.1.0-rc.1", isOrgReference: true},
		{input: "0xorg/pkg@v1.0.0", org: "0xorg", packageName: "pkg", version: "v1.0.0", isOrgReference: true},
		{input: "sf/uniswap-v4", org: "sf", packageName: "uniswap-v4", version: "latest", isOrgReference: true},
		{input: "sf/uniswap-v4@", org: "sf", packageName: "uniswap-v4", version: "latest", isOrgReference: true},
		{input: "sf/uniswap-v4@latest", org: "sf", packageName: "uniswap-v4", version: "latest", isOrgReference: true},

		{input: "uniswap-v4@v1.0.0"},
		{input: "uniswap-v4"},
		{input: "./sf/uniswap-v4"},
		{input: "../sf/uniswap-v4@v1.0.0"},
		{input: "/sf/uniswap-v4@v1.0.0"},
		{input: "a/b/c@v1.0.0"},
		{input: "abis/erc20.spkg"},
		{input: "My_Dir/pkg"},
		{input: "https://spkg.io/v1/packages/sf/pkg/v1.0.0"},

		{input: "Sf/uniswap-v4@v1.0.0", isOrgReference: true, expectError: "not a valid organization slug"},
		{input: "s/uniswap-v4@v1.0.0", isOrgReference: true, expectError: "not a valid organization slug"},
		{input: "my--org/pkg@v1.0.0", isOrgReference: true, expectError: "not a valid organization slug"},
		{input: "sf/1pkg@v1.0.0", isOrgReference: true, expectError: "does not match regexp"},
		{input: "sf/pkg@1.0.0", isOrgReference: true, expectError: "not valid Semver"},
		{input: "sf/pkg@v1.0.0@v2.0.0", isOrgReference: true, expectError: "not valid Semver"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			org, packageName, version, isOrgReference, err := ParseOrgPackageReference(tt.input)
			assert.Equal(t, tt.isOrgReference, isOrgReference)

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.org, org)
			assert.Equal(t, tt.packageName, packageName)
			assert.Equal(t, tt.version, version)
		})
	}
}

func TestReader_ResolveOrgPackageReferenceInput(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, "local", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "local", "pkg@v1.0.0"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "pkg@v1.0.0"), []byte("x"), 0o644))
	t.Chdir(workDir)

	const registry = "https://registry.test"

	tests := []struct {
		input       string
		want        string
		expectError string
	}{
		{input: "sf/uniswap-v4@v1.0.0", want: registry + "/v1/packages/sf/uniswap-v4/v1.0.0"},
		{input: "sf/uniswap-v4", want: registry + "/v1/packages/sf/uniswap-v4/latest"},
		{input: "sf/uniswap-v4@latest", want: registry + "/v1/packages/sf/uniswap-v4/latest"},
		{input: "uniswap-v4@v1.0.0", want: registry + "/v1/packages/uniswap-v4/v1.0.0"},
		{input: "uniswap-v4", want: registry + "/v1/packages/uniswap-v4/latest"},

		{input: "local/pkg", want: filepath.Join("local", "pkg", "substreams.yaml")},
		{input: "local/pkg@v1.0.0", want: "local/pkg@v1.0.0"},
		{input: "pkg@v1.0.0", want: registry + "/v1/packages/pkg/v1.0.0"},

		{input: "./sf/uniswap-v4", expectError: "unable to stat input file"},
		{input: "abis/erc20.spkg", expectError: "unable to stat input file"},
		{input: "Sf/uniswap-v4@v1.0.0", expectError: "invalid Substreams Registry package reference"},
		{input: "sf/uniswap-v4@1.0.0", expectError: "invalid Substreams Registry package reference"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := newReader(tt.input, workDir, WithRegistryURL(registry))
			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, r.currentInput)
		})
	}
}

func TestReader_ImportOrgPackageReference(t *testing.T) {
	spkg1Content, err := os.ReadFile("testdata/spkg1/spkg1-v0.0.0.spkg")
	require.NoError(t, err)

	var requestedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Write(spkg1Content)
	}))
	defer server.Close()

	tests := []struct {
		name          string
		importValue   string
		localFile     string
		wantRequested []string
		expectError   string
	}{
		{name: "org package with version", importValue: "sf/spkg1@v0.0.0", wantRequested: []string{"/v1/packages/sf/spkg1/v0.0.0"}},
		{name: "personal package with version", importValue: "spkg1@v0.0.0", wantRequested: []string{"/v1/packages/spkg1/v0.0.0"}},
		{name: "org package without version", importValue: "sf/spkg1", expectError: "needs a specific version"},
		{name: "org package at latest", importValue: "sf/spkg1@latest", expectError: "needs a specific version"},
		{name: "org package with invalid version", importValue: "sf/spkg1@1.0.0", expectError: "not valid Semver"},
		{name: "existing local path shaped like an org package", importValue: "sf/spkg1", localFile: "sf/spkg1"},
		{name: "explicit relative path", importValue: "./sf/spkg1", localFile: "sf/spkg1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestedPaths = nil

			dir := t.TempDir()
			if tt.localFile != "" {
				localPath := filepath.Join(dir, tt.localFile)
				require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0o755))
				require.NoError(t, os.WriteFile(localPath, spkg1Content, 0o644))
			}

			manifestPath := filepath.Join(dir, "substreams.yaml")
			require.NoError(t, os.WriteFile(manifestPath, []byte("specVersion: v0.1.0\npackage:\n  name: test\n  version: v0.0.0\nimports:\n  spkg1: "+tt.importValue+"\n"), 0o644))

			r, err := newReader(manifestPath, dir, WithRegistryURL(server.URL))
			require.NoError(t, err)

			bundle, err := r.Read()
			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Empty(t, requestedPaths)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRequested, requestedPaths)
			require.Len(t, bundle.Package.PackageMeta, 2)
			assert.Equal(t, "spkg1", bundle.Package.PackageMeta[1].Name)
		})
	}
}

func TestIsLikelyManifestInput_PackageReferences(t *testing.T) {
	t.Chdir(t.TempDir())

	assert.True(t, IsLikelyManifestInput("sf/uniswap-v4@v1.0.0"))
	assert.True(t, IsLikelyManifestInput("sf/uniswap-v4"))
	assert.False(t, IsLikelyManifestInput("map_events"))
	assert.False(t, IsLikelyManifestInput("localhost:9000"))
}

func TestReader_VersionlessOrgReferenceHintsLocalPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	r, err := newReader("myproj/subdir", t.TempDir(), WithRegistryURL(server.URL))
	require.NoError(t, err)

	_, err = r.Read()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package does not exist on the Substreams registry")
	assert.Contains(t, err.Error(), `no local path "myproj/subdir" either`)

	r, err = newReader("myproj/subdir@v1.0.0", t.TempDir(), WithRegistryURL(server.URL))
	require.NoError(t, err)

	_, err = r.Read()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "no local path")
}
