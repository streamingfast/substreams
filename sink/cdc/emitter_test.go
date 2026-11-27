package cdc

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmitter_Registry(t *testing.T) {
	called := false
	RegisterEmitter("testfake", func(_ context.Context, u *url.URL, opts EmitterOptions) (Emitter, error) {
		called = true
		assert.Equal(t, "host", u.Host)
		assert.NotNil(t, opts.Logger)
		return &stdoutEmitter{}, nil
	})

	em, err := NewEmitter(t.Context(), "testfake://host/path", EmitterOptions{})
	require.NoError(t, err)
	assert.NotNil(t, em)
	assert.True(t, called)

	assert.Panics(t, func() { RegisterEmitter("testfake", nil) })
	assert.Panics(t, func() {
		RegisterEmitter("testfake", func(context.Context, *url.URL, EmitterOptions) (Emitter, error) { return nil, nil })
	})
}

func TestNewEmitter_UnknownScheme(t *testing.T) {
	_, err := NewEmitter(t.Context(), "nope://x", EmitterOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no emitter for scheme "nope"`)
	assert.Contains(t, err.Error(), "stdout")
}

func TestStdoutEmitter(t *testing.T) {
	for _, raw := range []string{"stdout://", "-"} {
		var out bytes.Buffer
		em, err := NewEmitter(t.Context(), raw, EmitterOptions{Stdout: &out})
		require.NoError(t, err)

		batch := &Batch{Events: []Event{newEvent("a", OpCreate, 1, "t"), newEvent("b", OpCreate, 1, "t")}}
		require.NoError(t, em.EmitBatch(t.Context(), batch))
		require.NoError(t, em.EmitCompensation(t.Context(), &Batch{Events: []Event{newEvent("c", OpDelete, 1, "t")}}))
		require.NoError(t, em.Close())

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		require.Len(t, lines, 3, raw)
		assert.Contains(t, lines[0], `"id":"a"`)
		assert.Contains(t, lines[2], `"op":"d"`)
	}
}
