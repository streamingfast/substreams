package devenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeOneBlock(t *testing.T, tmpDir, name string) {
	t.Helper()
	dir := filepath.Join(tmpDir, "one-blocks")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o644))
}

func TestWaitBurstProduced_ReturnsOnceBurstIsReached(t *testing.T) {
	tmpDir := t.TempDir()
	writeOneBlock(t, tmpDir, "0000000023-535fa30d7e25dd8a-3e7b4f1a92c0d6e5-20-default.dbin.zst")

	go func() {
		time.Sleep(200 * time.Millisecond)
		writeOneBlock(t, tmpDir, "0000000501-9e6a72557ada15d0-db3defda18fafc0c-490-default.dbin.zst")
	}()

	require.NoError(t, waitBurstProduced(context.Background(), tmpDir, 500, 5*time.Second))
}

func TestWaitBurstProduced_TimesOutMidBurst(t *testing.T) {
	tmpDir := t.TempDir()
	writeOneBlock(t, tmpDir, "0000000023-535fa30d7e25dd8a-3e7b4f1a92c0d6e5-20-default.dbin.zst")

	err := waitBurstProduced(context.Background(), tmpDir, 500, 300*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "highest one-block file is #23")
}

func TestWaitBurstProduced_NoBurst(t *testing.T) {
	require.NoError(t, waitBurstProduced(context.Background(), t.TempDir(), 0, time.Millisecond))
}
