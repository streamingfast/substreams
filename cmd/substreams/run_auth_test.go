package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/streamingfast/substreams/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clientConfig(endpoint string, authType client.AuthType, plaintext bool) *client.SubstreamsClientConfig {
	token := ""
	if authType != client.None {
		token = "some-credential"
	}
	return client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
		Endpoint:  endpoint,
		AuthToken: token,
		AuthType:  authType,
		PlainText: plaintext,
	})
}

func TestNeedsAuthPrompt(t *testing.T) {
	for name, test := range map[string]struct {
		cfg  *client.SubstreamsClientConfig
		want bool
	}{
		"streamingfast endpoint, no credentials": {clientConfig("mainnet.eth.streamingfast.io:443", client.None, false), true},
		"devnet endpoint, no credentials":        {clientConfig("devnet.sol.streamingfast.io:443", client.None, false), true},
		"api key already set":                    {clientConfig("mainnet.eth.streamingfast.io:443", client.ApiKey, false), false},
		"jwt already set":                        {clientConfig("mainnet.eth.streamingfast.io:443", client.JWT, false), false},
		"plaintext, a local or dev endpoint":     {clientConfig("localhost:9000", client.None, true), false},
		"self-hosted endpoint":                   {clientConfig("substreams.example.com:443", client.None, false), false},
		"look-alike host":                        {clientConfig("streamingfast.io.example.com:443", client.None, false), false},
		"no config":                              {nil, false},
	} {
		assert.Equal(t, test.want, needsAuthPrompt(test.cfg), name)
	}
}

type promptRecorder struct {
	confirmed     int
	authenticated int
	written       map[string]string
	env           map[string]string
}

func newPrompter(rec *promptRecorder, interactive, answer bool, authErr error) authPrompter {
	rec.written = map[string]string{}
	rec.env = map[string]string{}
	return authPrompter{
		interactive: func() bool { return interactive },
		confirm: func(string) (bool, error) {
			rec.confirmed++
			return answer, nil
		},
		authenticate: func(context.Context, io.Writer) (string, error) {
			rec.authenticated++
			return "  server_selected-key\n", authErr
		},
		writeCredentials: func(_ io.Writer, envvar string, apiKey string) error {
			rec.written[envvar] = apiKey
			return nil
		},
		setenv: func(key, value string) error {
			rec.env[key] = value
			return nil
		},
		out: io.Discard,
	}
}

func TestMaybeAuthenticate(t *testing.T) {
	ctx := context.Background()
	unauthenticated := clientConfig("mainnet.eth.streamingfast.io:443", client.None, false)

	t.Run("yes runs the auth flow, writes .substreams.env and sets the key for this run", func(t *testing.T) {
		rec := &promptRecorder{}
		done, err := newPrompter(rec, true, true, nil).maybeAuthenticate(ctx, unauthenticated, "MY_KEY_VAR")
		require.NoError(t, err)
		assert.True(t, done)
		assert.Equal(t, 1, rec.authenticated)
		assert.Contains(t, rec.written, "MY_KEY_VAR", ".substreams.env stores the key under the variable the run reads")
		assert.Equal(t, map[string]string{"MY_KEY_VAR": "server_selected-key"}, rec.env, "the --api-key-envvar name, trimmed")
	})

	t.Run("no leaves everything as is", func(t *testing.T) {
		rec := &promptRecorder{}
		done, err := newPrompter(rec, true, false, nil).maybeAuthenticate(ctx, unauthenticated, "SUBSTREAMS_API_KEY")
		require.NoError(t, err)
		assert.False(t, done)
		assert.Equal(t, 1, rec.confirmed)
		assert.Zero(t, rec.authenticated)
		assert.Empty(t, rec.env)
	})

	t.Run("nobody at a terminal is never asked", func(t *testing.T) {
		rec := &promptRecorder{}
		done, err := newPrompter(rec, false, true, nil).maybeAuthenticate(ctx, unauthenticated, "SUBSTREAMS_API_KEY")
		require.NoError(t, err)
		assert.False(t, done)
		assert.Zero(t, rec.confirmed)
	})

	t.Run("credentials already set are never asked about", func(t *testing.T) {
		rec := &promptRecorder{}
		authed := clientConfig("mainnet.eth.streamingfast.io:443", client.ApiKey, false)
		done, err := newPrompter(rec, true, true, nil).maybeAuthenticate(ctx, authed, "SUBSTREAMS_API_KEY")
		require.NoError(t, err)
		assert.False(t, done)
		assert.Zero(t, rec.confirmed)
	})

	t.Run("a failed or cancelled login is returned and changes nothing", func(t *testing.T) {
		rec := &promptRecorder{}
		done, err := newPrompter(rec, true, true, errors.New("login cancelled")).maybeAuthenticate(ctx, unauthenticated, "SUBSTREAMS_API_KEY")
		assert.EqualError(t, err, "login cancelled")
		assert.False(t, done)
		assert.Empty(t, rec.written)
		assert.Empty(t, rec.env)
	})
}
