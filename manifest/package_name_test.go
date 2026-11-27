package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePackageName(t *testing.T) {
	tests := []struct {
		name        string
		expectError string
	}{
		{name: "uniswap-v4"},
		{name: "my_pkg"},
		{name: "sf/uniswap-v4"},
		{name: "sf/foo-bar"},
		{name: "my-org/my_pkg"},
		{name: "0xorg/pkg"},

		{name: "", expectError: "must match"},
		{name: "1pkg", expectError: "must match"},
		{name: "-pkg", expectError: "must match"},
		{name: strings.Repeat("a", 65), expectError: "must match"},
		{name: "Sf/pkg", expectError: "not a valid organization slug"},
		{name: "s/pkg", expectError: "not a valid organization slug"},
		{name: "my--org/pkg", expectError: "not a valid organization slug"},
		{name: "my_org/pkg", expectError: "not a valid organization slug"},
		{name: "/pkg", expectError: "not a valid organization slug"},
		{name: "sf/", expectError: "does not match regexp"},
		{name: "sf/1pkg", expectError: "does not match regexp"},
		{name: "sf/a/b", expectError: "does not match regexp"},
		{name: "sf/pkg@v1.0.0", expectError: "does not match regexp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageName(tt.name)
			if tt.expectError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestPackageNameFileSlug(t *testing.T) {
	assert.Equal(t, "uniswap-v4", PackageNameFileSlug("uniswap-v4"))
	assert.Equal(t, "my-pkg", PackageNameFileSlug("my_pkg"))
	assert.Equal(t, "sf-uniswap-v4", PackageNameFileSlug("sf/uniswap-v4"))
	assert.Equal(t, "my-org-my-pkg", PackageNameFileSlug("my-org/my_pkg"))
}

func TestReader_Read_PackageName(t *testing.T) {
	tests := []struct {
		name        string
		pkgName     string
		expectError string
	}{
		{name: "bare", pkgName: "foo-bar"},
		{name: "org scoped", pkgName: "sf/foo-bar"},
		{name: "invalid org", pkgName: "SF/foo-bar", expectError: "not a valid organization slug"},
		{name: "invalid name", pkgName: "sf/1foo", expectError: "does not match regexp"},
		{name: "nested", pkgName: "sf/foo/bar", expectError: "does not match regexp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "substreams.yaml")
			content := "specVersion: v0.1.0\npackage:\n  name: " + tt.pkgName + "\n  version: v0.1.0\nmodules: []\n"
			require.NoError(t, os.WriteFile(path, []byte(content), 0644))

			reader, err := NewReader(path)
			require.NoError(t, err)

			bundle, err := reader.Read()
			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.pkgName, bundle.Package.PackageMeta[0].Name)
		})
	}
}
