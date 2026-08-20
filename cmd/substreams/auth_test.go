package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortalAPIBaseURL(t *testing.T) {
	t.Run("default production", func(t *testing.T) {
		t.Setenv("SUBSTREAMS_PORTAL_API", "")
		t.Setenv("LOCAL_DEVELOPMENT", "")
		assert.Equal(t, defaultPortalAPIURL, portalAPIBaseURL())
	})

	t.Run("local development", func(t *testing.T) {
		t.Setenv("SUBSTREAMS_PORTAL_API", "")
		t.Setenv("LOCAL_DEVELOPMENT", "true")
		assert.Equal(t, localPortalAPIURL, portalAPIBaseURL())
	})

	t.Run("explicit override wins", func(t *testing.T) {
		t.Setenv("LOCAL_DEVELOPMENT", "true")
		t.Setenv("SUBSTREAMS_PORTAL_API", "http://example.test:9000/")
		assert.Equal(t, "http://example.test:9000", portalAPIBaseURL())
	})
}

func TestWriteAuthEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".substreams.env")

	require.NoError(t, writeAuthEnvFile(path, authCredentials{token: "jwt-token"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "export SUBSTREAMS_API_TOKEN=jwt-token\n", string(got))
}

func TestWriteAuthEnvFileAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".substreams.env")

	require.NoError(t, writeAuthEnvFile(path, authCredentials{apiKey: "server_abc"}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "export SUBSTREAMS_API_KEY=server_abc\n", string(got))
}

func TestWriteAuthEnvFileChmodsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".substreams.env")
	require.NoError(t, os.WriteFile(path, []byte("export OLD=1\n"), 0o644))

	require.NoError(t, writeAuthEnvFile(path, authCredentials{token: "jwt-token"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "export SUBSTREAMS_API_TOKEN=jwt-token\n", string(got))
}

func TestAuthIssueBaseURL(t *testing.T) {
	t.Setenv("SUBSTREAMS_AUTH_ISSUE_URL", "")
	t.Setenv("LOCAL_DEVELOPMENT", "")
	assert.Equal(t, defaultAuthBaseURL, authIssueBaseURL())

	t.Setenv("LOCAL_DEVELOPMENT", "true")
	assert.Equal(t, localAuthBaseURL, authIssueBaseURL())

	t.Setenv("SUBSTREAMS_AUTH_ISSUE_URL", "http://issuer.test/")
	assert.Equal(t, "http://issuer.test", authIssueBaseURL())
}

func TestResolveCredentialsPassthrough(t *testing.T) {
	got, err := resolveCredentials("eyJhbGciOiJIUzI1NiJ9.e30.ok")
	require.NoError(t, err)
	assert.Equal(t, "eyJhbGciOiJIUzI1NiJ9.e30.ok", got.token)
	assert.Empty(t, got.apiKey)
}

func TestResolveCredentialsLocalFallback(t *testing.T) {
	t.Setenv("LOCAL_DEVELOPMENT", "true")
	t.Setenv("SUBSTREAMS_AUTH_ISSUE_URL", "http://127.0.0.1:1")

	got, err := resolveCredentials("server_abc")
	require.NoError(t, err)
	assert.Empty(t, got.token)
	assert.Equal(t, "server_abc", got.apiKey)
}

func TestRedactSecrets(t *testing.T) {
	in := `{"code":"auth_unauthorized_api_key_error","details":{"api_key":"server_deadbeef0123456789"}}`
	got := redactSecrets(in)
	assert.NotContains(t, got, "server_deadbeef0123456789")
	assert.Contains(t, got, "[redacted]")
}

func TestRedactSecretsDeviceCode(t *testing.T) {
	in := `{"device_code":"secret-device","deviceCode":"camel-device","message":"bad"}`
	got := redactSecrets(in)
	assert.NotContains(t, got, "secret-device")
	assert.NotContains(t, got, "camel-device")
	assert.Contains(t, got, `"device_code":"[redacted]"`)
	assert.Contains(t, got, `"deviceCode":"[redacted]"`)
	assert.Contains(t, got, "bad")
}

func TestPortalClientAuthorizeCamelCase(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sf.portalapi.v1.PortalApi/CliApiKeyAuthorize", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "1", r.Header.Get("Connect-Protocol-Version"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = body
		_, _ = w.Write([]byte(`{
			"deviceCode": "device_secret",
			"userCode": "ABCD-1234",
			"verificationUri": "http://localhost:3000/auth/cli",
			"verificationUriComplete": "http://localhost:3000/auth/cli?user_code=ABCD-1234",
			"expiresIn": "600",
			"interval": "5"
		}`))
	}))
	t.Cleanup(srv.Close)

	got, err := newPortalClient(srv.URL).CliAPIKeyAuthorize(t.Context(), "substreams CLI")
	require.NoError(t, err)

	assert.JSONEq(t, `{"client_name":"substreams CLI"}`, string(gotBody))
	assert.Equal(t, "device_secret", got.DeviceCode)
	assert.Equal(t, "ABCD-1234", got.UserCode)
	assert.Equal(t, "http://localhost:3000/auth/cli", got.VerificationURI)
	assert.Equal(t, "http://localhost:3000/auth/cli?user_code=ABCD-1234", got.VerificationURIComplete)
	assert.Equal(t, 10*time.Minute, got.ExpiresIn)
	assert.Equal(t, 5*time.Second, got.Interval)
}

func TestPortalClientAuthorizeSnakeCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"device_code": "device_secret",
			"user_code": "ABCD-1234",
			"verification_uri": "http://localhost:3000/auth/cli",
			"verification_uri_complete": "http://localhost:3000/auth/cli?user_code=ABCD-1234",
			"expires_in": 600,
			"interval": 5
		}`))
	}))
	t.Cleanup(srv.Close)

	got, err := newPortalClient(srv.URL).CliAPIKeyAuthorize(t.Context(), "cli")
	require.NoError(t, err)
	assert.Equal(t, "device_secret", got.DeviceCode)
	assert.Equal(t, 10*time.Minute, got.ExpiresIn)
}

func TestPortalClientTokenApproved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sf.portalapi.v1.PortalApi/CliApiKeyToken", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"device_code":"device_secret"}`, string(body))
		_, _ = w.Write([]byte(`{
			"status": "CLI_API_KEY_TOKEN_STATUS_APPROVED",
			"apiKey": "server_abc",
			"apiKeyId": "key-1",
			"apiKeyName": "dev",
			"organizationId": "org-1"
		}`))
	}))
	t.Cleanup(srv.Close)

	got, err := newPortalClient(srv.URL).CliAPIKeyToken(t.Context(), "device_secret")
	require.NoError(t, err)
	assert.Equal(t, cliAPIKeyApproved, got.Status)
	assert.Equal(t, "server_abc", got.APIKey)
	assert.Equal(t, "key-1", got.APIKeyID)
	assert.Equal(t, "dev", got.APIKeyName)
	assert.Equal(t, "org-1", got.OrganizationID)
}

func TestPortalClientTokenNumericStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	t.Cleanup(srv.Close)

	got, err := newPortalClient(srv.URL).CliAPIKeyToken(t.Context(), "code")
	require.NoError(t, err)
	assert.Equal(t, cliAPIKeyPending, got.Status)
}

func TestPortalClientConnectError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_argument","message":"unknown device code"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := newPortalClient(srv.URL).CliAPIKeyToken(t.Context(), "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown device code")
	assert.Contains(t, err.Error(), "invalid_argument")
}

func TestPortalClientConnectErrorRedactsDeviceCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`not-json {"device_code":"secret-device"} leftover`))
	}))
	t.Cleanup(srv.Close)

	_, err := newPortalClient(srv.URL).CliAPIKeyToken(t.Context(), "secret-device")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-device")
	assert.Contains(t, err.Error(), `"device_code":"[redacted]"`)
}

func TestParseCLIAPIKeyStatus(t *testing.T) {
	assert.Equal(t, cliAPIKeyApproved, parseCLIAPIKeyStatus("CLI_API_KEY_TOKEN_STATUS_APPROVED"))
	assert.Equal(t, cliAPIKeyPending, parseCLIAPIKeyStatus("pending"))
	assert.Equal(t, cliAPIKeySlowDown, parseCLIAPIKeyStatus("SLOW_DOWN"))
	assert.Equal(t, cliAPIKeyUnknown, parseCLIAPIKeyStatus("CLI_API_KEY_TOKEN_STATUS_UNSPECIFIED"))
}

func TestWaitForCLIAPIKeyApprovedAfterPending(t *testing.T) {
	fake := &scriptedTokener{results: []cliAPIKeyTokenResult{
		{Status: cliAPIKeyPending},
		{Status: cliAPIKeyApproved, APIKey: "server_abc", APIKeyName: "dev"},
	}}
	var sleeps []time.Duration

	got, err := waitForCLIAPIKey(t.Context(), fake, "code", 5*time.Second, time.Minute, func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "server_abc", got.APIKey)
	assert.Equal(t, []time.Duration{5 * time.Second}, sleeps)
	assert.Equal(t, 2, fake.calls)
}

func TestWaitForCLIAPIKeySlowDownIncreasesInterval(t *testing.T) {
	fake := &scriptedTokener{results: []cliAPIKeyTokenResult{
		{Status: cliAPIKeySlowDown},
		{Status: cliAPIKeyApproved, APIKey: "server_abc"},
	}}
	var sleeps []time.Duration

	got, err := waitForCLIAPIKey(t.Context(), fake, "code", 5*time.Second, time.Minute, func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "server_abc", got.APIKey)
	assert.Equal(t, []time.Duration{10 * time.Second}, sleeps)
}

func TestWaitForCLIAPIKeyDenied(t *testing.T) {
	fake := &scriptedTokener{results: []cliAPIKeyTokenResult{
		{Status: cliAPIKeyDenied},
	}}
	_, err := waitForCLIAPIKey(t.Context(), fake, "code", time.Second, time.Minute, func(context.Context, time.Duration) error {
		require.Fail(t, "should not sleep after deny")
		return nil
	})
	require.EqualError(t, err, "login was denied")
}

func TestWaitForCLIAPIKeyExpiredStatus(t *testing.T) {
	fake := &scriptedTokener{results: []cliAPIKeyTokenResult{
		{Status: cliAPIKeyExpired},
	}}
	_, err := waitForCLIAPIKey(t.Context(), fake, "code", time.Second, time.Minute, nil)
	require.EqualError(t, err, "login expired before a key was selected")
}

func TestWaitForCLIAPIKeyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fake := &scriptedTokener{results: []cliAPIKeyTokenResult{
		{Status: cliAPIKeyPending},
	}}
	_, err := waitForCLIAPIKey(ctx, fake, "code", time.Second, time.Minute, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitForCLIAPIKeyDeadline(t *testing.T) {
	fake := &scriptedTokener{results: []cliAPIKeyTokenResult{
		{Status: cliAPIKeyPending},
	}}
	_, err := waitForCLIAPIKey(t.Context(), fake, "code", time.Second, 0, func(context.Context, time.Duration) error {
		require.Fail(t, "should not sleep after deadline")
		return nil
	})
	require.EqualError(t, err, "login expired before a key was selected")
}

func TestWaitForCLIAPIKeyRetries5xxThenSucceeds(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":"unavailable","message":"try again"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"APPROVED","apiKey":"server_abc"}`))
	}))
	t.Cleanup(srv.Close)

	got, err := waitForCLIAPIKey(t.Context(), newPortalClient(srv.URL), "code", time.Millisecond, time.Minute, func(context.Context, time.Duration) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "server_abc", got.APIKey)
	assert.Equal(t, 2, int(n.Load()))
}

func TestWaitForCLIAPIKeyFailsFastOn4xx(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_argument","message":"unknown device code"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := waitForCLIAPIKey(t.Context(), newPortalClient(srv.URL), "code", time.Second, time.Minute, func(context.Context, time.Duration) error {
		require.Fail(t, "should not sleep after 4xx")
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "polling login")
	assert.Equal(t, 1, int(n.Load()))
}

func TestWaitForCLIAPIKeyRetriesTransportThenSucceeds(t *testing.T) {
	var calls int
	tok := &funcTokener{fn: func() (*cliAPIKeyTokenResult, error) {
		calls++
		if calls == 1 {
			return nil, &portalAPIError{retryable: true, err: errors.New("connection refused")}
		}
		return &cliAPIKeyTokenResult{Status: cliAPIKeyApproved, APIKey: "server_abc"}, nil
	}}

	got, err := waitForCLIAPIKey(t.Context(), tok, "code", time.Millisecond, time.Minute, func(context.Context, time.Duration) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "server_abc", got.APIKey)
	assert.Equal(t, 2, calls)
}

func TestWaitForCLIAPIKeyRetriesTimeoutThenSucceeds(t *testing.T) {
	var calls int
	tok := &funcTokener{fn: func() (*cliAPIKeyTokenResult, error) {
		calls++
		if calls == 1 {
			return nil, &portalAPIError{
				retryable: true,
				err:       fmt.Errorf("CliApiKeyToken request: %w", context.DeadlineExceeded),
			}
		}
		return &cliAPIKeyTokenResult{Status: cliAPIKeyApproved, APIKey: "server_abc"}, nil
	}}

	got, err := waitForCLIAPIKey(t.Context(), tok, "code", time.Millisecond, time.Minute, func(context.Context, time.Duration) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "server_abc", got.APIKey)
	assert.Equal(t, 2, calls)
}

func TestWaitForCLIAPIKeyDoesNotRetryCanceled(t *testing.T) {
	tok := &funcTokener{fn: func() (*cliAPIKeyTokenResult, error) {
		return nil, &portalAPIError{
			retryable: false,
			err:       fmt.Errorf("CliApiKeyToken request: %w", context.Canceled),
		}
	}}
	_, err := waitForCLIAPIKey(t.Context(), tok, "code", time.Second, time.Minute, func(context.Context, time.Duration) error {
		require.Fail(t, "should not sleep after cancel")
		return nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestVerificationURLComplete(t *testing.T) {
	got, err := verificationURL(&cliAPIKeyAuthorizeResult{
		VerificationURIComplete: "https://thegraph.market/auth/cli?user_code=ABCD-1234",
		VerificationURI:         "https://thegraph.market/auth/cli",
		UserCode:                "ABCD-1234",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://thegraph.market/auth/cli?user_code=ABCD-1234", got)
}

func TestVerificationURLFallback(t *testing.T) {
	got, err := verificationURL(&cliAPIKeyAuthorizeResult{
		VerificationURI: "http://localhost:3000/auth/cli",
		UserCode:        "ABCD-1234",
	})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000/auth/cli?user_code=ABCD-1234", got)
}

func TestVerificationURLQueryEscapesUserCode(t *testing.T) {
	got, err := verificationURL(&cliAPIKeyAuthorizeResult{
		VerificationURI: "http://localhost:3000/auth/cli",
		UserCode:        "A B",
	})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000/auth/cli?user_code=A+B", got)
}

func TestVerificationURLMissing(t *testing.T) {
	_, err := verificationURL(&cliAPIKeyAuthorizeResult{DeviceCode: "device_secret"})
	require.EqualError(t, err, "missing http(s) verification URL")
}

func TestVerificationURLRejectsNonHTTP(t *testing.T) {
	_, err := verificationURL(&cliAPIKeyAuthorizeResult{
		VerificationURIComplete: "javascript:alert(1)",
	})
	require.EqualError(t, err, "missing http(s) verification URL")
}

func TestRunCliAPIKeyAuthRequiresHTTPVerificationURL(t *testing.T) {
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sf.portalapi.v1.PortalApi/CliApiKeyAuthorize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deviceCode": "device_secret",
				"expiresIn":  "600",
				"interval":   "1",
			})
		case "/sf.portalapi.v1.PortalApi/CliApiKeyToken":
			tokenCalls.Add(1)
			http.Error(w, "should not poll", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	var opened string
	_, err := runCliAPIKeyAuth(t.Context(), newPortalClient(srv.URL), io.Discard, func(u string) error {
		opened = u
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing http(s) verification URL")
	assert.Equal(t, 0, int(tokenCalls.Load()))
	assert.Empty(t, opened)
}

func TestRunCliAPIKeyAuthSuccess(t *testing.T) {
	var authorizeCalls, tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sf.portalapi.v1.PortalApi/CliApiKeyAuthorize":
			authorizeCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deviceCode":              "device_secret",
				"userCode":                "ABCD-1234",
				"verificationUri":         "http://localhost:3000/auth/cli",
				"verificationUriComplete": "http://localhost:3000/auth/cli?user_code=ABCD-1234",
				"expiresIn":               "600",
				"interval":                "1",
			})
		case "/sf.portalapi.v1.PortalApi/CliApiKeyToken":
			tokenCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":         "CLI_API_KEY_TOKEN_STATUS_APPROVED",
				"apiKey":         "server_abc",
				"apiKeyId":       "key-1",
				"apiKeyName":     "dev",
				"organizationId": "org-1",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	var opened string
	var out strings.Builder
	got, err := runCliAPIKeyAuth(t.Context(), newPortalClient(srv.URL), &out, func(u string) error {
		opened = u
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, int(authorizeCalls.Load()))
	assert.Equal(t, 1, int(tokenCalls.Load()))
	assert.Equal(t, "http://localhost:3000/auth/cli?user_code=ABCD-1234", opened)
	assert.Equal(t, "server_abc", got)
	assert.Contains(t, out.String(), "ABCD-1234")
	assert.Contains(t, out.String(), "Opened the key picker")
	assert.Contains(t, out.String(), `"dev"`)
	assert.NotContains(t, out.String(), "device_secret")
	assert.NotContains(t, out.String(), "server_abc")
}

type funcTokener struct {
	fn func() (*cliAPIKeyTokenResult, error)
}

func (f *funcTokener) CliAPIKeyToken(ctx context.Context, deviceCode string) (*cliAPIKeyTokenResult, error) {
	return f.fn()
}

type scriptedTokener struct {
	results []cliAPIKeyTokenResult
	calls   int
}

func (s *scriptedTokener) CliAPIKeyToken(ctx context.Context, deviceCode string) (*cliAPIKeyTokenResult, error) {
	if s.calls >= len(s.results) {
		result := s.results[len(s.results)-1]
		s.calls++
		return &result, ctx.Err()
	}
	result := s.results[s.calls]
	s.calls++
	return &result, nil
}
