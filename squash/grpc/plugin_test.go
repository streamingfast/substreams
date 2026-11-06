package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePluginURL(t *testing.T) {
	endpoint, opts, err := parsePluginURL("grpc://localhost:9002")
	require.NoError(t, err)
	assert.Equal(t, "localhost:9002", endpoint)
	assert.True(t, opts.PlainText)

	endpoint, opts, err = parsePluginURL("grpc://localhost:9002?plaintext=false&insecure=true&secret=test-secret")
	require.NoError(t, err)
	assert.Equal(t, "localhost:9002", endpoint)
	assert.False(t, opts.PlainText)
	assert.True(t, opts.Insecure)
	assert.Equal(t, "test-secret", opts.Secret)

	endpoint, opts, err = parsePluginURL("grpcs://squasher.example:443")
	require.NoError(t, err)
	assert.Equal(t, "squasher.example:443", endpoint)
	assert.False(t, opts.PlainText)

	endpoint, opts, err = parsePluginURL("grpcs://squasher.example?secret=test-secret")
	require.NoError(t, err)
	assert.Equal(t, "squasher.example:443", endpoint)
	assert.False(t, opts.PlainText)
	assert.False(t, opts.Insecure)
	assert.Equal(t, "test-secret", opts.Secret)

	endpoint, opts, err = parsePluginURL("grpcs://squasher.example:8443?secret=test-secret")
	require.NoError(t, err)
	assert.Equal(t, "squasher.example:8443", endpoint)
	assert.False(t, opts.PlainText)

	_, _, err = parsePluginURL("grpc://localhost")
	require.Error(t, err)

	_, _, err = parsePluginURL("grpc://")
	require.Error(t, err)
}
