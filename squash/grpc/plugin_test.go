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

	endpoint, opts, err = parsePluginURL("grpc://localhost:9002?plaintext=false&insecure=true&secret=s3cret")
	require.NoError(t, err)
	assert.Equal(t, "localhost:9002", endpoint)
	assert.False(t, opts.PlainText)
	assert.True(t, opts.Insecure)
	assert.Equal(t, "s3cret", opts.Secret)

	endpoint, opts, err = parsePluginURL("grpcs://squasher.internal:443")
	require.NoError(t, err)
	assert.Equal(t, "squasher.internal:443", endpoint)
	assert.False(t, opts.PlainText)

	// Same shape as OHV tier1 → GCloud tier2 (https://host, TLS :443, secret).
	endpoint, opts, err = parsePluginURL("grpcs://squasher.ovh2gcp.streamingfast.io?secret=s3cret")
	require.NoError(t, err)
	assert.Equal(t, "squasher.ovh2gcp.streamingfast.io:443", endpoint)
	assert.False(t, opts.PlainText)
	assert.False(t, opts.Insecure)
	assert.Equal(t, "s3cret", opts.Secret)

	endpoint, opts, err = parsePluginURL("grpcs://squasher.ovh2gcp.streamingfast.io:8443?secret=s3cret")
	require.NoError(t, err)
	assert.Equal(t, "squasher.ovh2gcp.streamingfast.io:8443", endpoint)
	assert.False(t, opts.PlainText)

	_, _, err = parsePluginURL("grpc://localhost")
	require.Error(t, err)

	_, _, err = parsePluginURL("grpc://")
	require.Error(t, err)
}
