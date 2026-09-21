package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteRegistryTokenCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".config", "substreams", "registry-token")
	prev := registryTokenFilename
	registryTokenFilename = path
	t.Cleanup(func() { registryTokenFilename = prev })

	require.NoError(t, writeRegistryToken("tok"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "tok", string(got))
}

func TestWriteRegistryTokenChmodsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry-token")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))
	prev := registryTokenFilename
	registryTokenFilename = path
	t.Cleanup(func() { registryTokenFilename = prev })

	require.NoError(t, writeRegistryToken("new"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
