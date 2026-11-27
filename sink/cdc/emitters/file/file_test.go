package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/streamingfast/substreams/sink/cdc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func batch(block uint64, ids ...string) *cdc.Batch {
	b := &cdc.Batch{Block: cdc.Block{Number: block, ID: fmt.Sprintf("b%d", block)}}
	for _, id := range ids {
		b.Events = append(b.Events, cdc.Event{ID: id, Op: cdc.OpCreate, Key: map[string]string{"id": id}})
	}
	return b
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func ids(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev cdc.Event
		require.NoError(t, json.Unmarshal(sc.Bytes(), &ev))
		out = append(out, ev.ID)
	}
	return out
}

func TestEmitter_RotatesByBlockRange(t *testing.T) {
	dir := t.TempDir()
	e, err := New(Config{Dir: dir, Blocks: 10, Prefix: "x_"}, zlog)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, e.EmitBatch(ctx, batch(1, "a")))
	// Durable and visible right after EmitBatch, as a part file.
	assert.Equal(t, []string{"x_0000000000-0000000009.part"}, names(t, dir))
	assert.Equal(t, []string{"a"}, ids(t, filepath.Join(dir, "x_0000000000-0000000009.part")))

	require.NoError(t, e.EmitBatch(ctx, batch(9, "b")))
	require.NoError(t, e.EmitBatch(ctx, batch(10, "c")))
	assert.Equal(t, []string{"x_0000000000-0000000009.jsonl", "x_0000000010-0000000019.part"}, names(t, dir))
	assert.Equal(t, []string{"a", "b"}, ids(t, filepath.Join(dir, "x_0000000000-0000000009.jsonl")))

	// Compensation is appended to the open file, without rotating back.
	require.NoError(t, e.EmitCompensation(ctx, batch(5, "undo")))
	assert.Equal(t, []string{"c", "undo"}, ids(t, filepath.Join(dir, "x_0000000010-0000000019.part")))
	require.NoError(t, e.Close())
	assert.Equal(t, []string{"x_0000000000-0000000009.jsonl", "x_0000000010-0000000019.part"}, names(t, dir))
}

func TestEmitter_ResumesLeftoverPart(t *testing.T) {
	dir := t.TempDir()
	e, err := New(Config{Dir: dir, Blocks: 10}, zlog)
	require.NoError(t, err)
	require.NoError(t, e.EmitBatch(t.Context(), batch(12, "a")))
	// crash: no Close

	e2, err := New(Config{Dir: dir, Blocks: 10}, zlog)
	require.NoError(t, err)
	require.NoError(t, e2.EmitBatch(t.Context(), batch(13, "b")))
	assert.Equal(t, []string{"a", "b"}, ids(t, filepath.Join(dir, "0000000010-0000000019.part")))
}

func TestEmitter_FinalizeOnClose(t *testing.T) {
	dir := t.TempDir()
	e, err := New(Config{Dir: dir, Blocks: 100, Finalize: true}, zlog)
	require.NoError(t, err)
	require.NoError(t, e.EmitBatch(t.Context(), batch(3, "a")))
	require.NoError(t, e.EmitBatch(t.Context(), batch(7, "b")))
	require.NoError(t, e.Close())
	assert.Equal(t, []string{"0000000003-0000000007.jsonl"}, names(t, dir))
}

func TestEmitter_RejectsDifferentBlocksPart(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "0000000000-0000000009.part"), nil, 0o644))
	_, err := New(Config{Dir: dir, Blocks: 20}, zlog)
	require.Error(t, err)
}

func TestFactory(t *testing.T) {
	dir := t.TempDir()
	e, err := cdc.NewEmitter(t.Context(), "file://"+dir+"?blocks=5&prefix=p_", cdc.EmitterOptions{})
	require.NoError(t, err)
	require.NoError(t, e.Close())

	_, err = cdc.NewEmitter(t.Context(), "file://"+dir+"?blocks=0", cdc.EmitterOptions{})
	require.Error(t, err)
	_, err = cdc.NewEmitter(t.Context(), "file://"+dir+"?blokcs=5", cdc.EmitterOptions{})
	require.ErrorContains(t, err, "unknown")
}
