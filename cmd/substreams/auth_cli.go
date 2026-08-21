package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	portalAPIPrefix        = "/sf.portalapi.v1.PortalApi/"
	defaultDevicePollWait  = 5 * time.Second
	slowDownExtraWait      = 5 * time.Second
	connectProtocolVersion = "1"
)

type cliAPIKeyStatus string

const (
	cliAPIKeyPending  cliAPIKeyStatus = "PENDING"
	cliAPIKeySlowDown cliAPIKeyStatus = "SLOW_DOWN"
	cliAPIKeyDenied   cliAPIKeyStatus = "DENIED"
	cliAPIKeyExpired  cliAPIKeyStatus = "EXPIRED"
	cliAPIKeyApproved cliAPIKeyStatus = "APPROVED"
	cliAPIKeyUnknown  cliAPIKeyStatus = "UNKNOWN"
)

type cliAPIKeyAuthorizeResult struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
}

type cliAPIKeyTokenResult struct {
	Status         cliAPIKeyStatus
	APIKey         string
	APIKeyID       string
	APIKeyName     string
	OrganizationID string
}

type cliAPIKeyTokener interface {
	CliAPIKeyToken(ctx context.Context, deviceCode string) (*cliAPIKeyTokenResult, error)
}

type portalClient struct {
	baseURL    string
	httpClient *http.Client
}

func newPortalClient(baseURL string) *portalClient {
	return &portalClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *portalClient) CliAPIKeyAuthorize(ctx context.Context, clientName string) (*cliAPIKeyAuthorizeResult, error) {
	body, err := c.post(ctx, "CliApiKeyAuthorize", map[string]string{
		"client_name": clientName,
	})
	if err != nil {
		return nil, err
	}

	var wire cliAPIKeyAuthorizeWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decoding CliApiKeyAuthorize response: %w", err)
	}

	result := &cliAPIKeyAuthorizeResult{
		DeviceCode:              firstNonEmpty(wire.DeviceCode, wire.DeviceCodeSnake),
		UserCode:                firstNonEmpty(wire.UserCode, wire.UserCodeSnake),
		VerificationURI:         firstNonEmpty(wire.VerificationURI, wire.VerificationURISnake),
		VerificationURIComplete: firstNonEmpty(wire.VerificationURIComplete, wire.VerificationURICompleteSnake),
		ExpiresIn:               time.Duration(wire.ExpiresIn.Int64()) * time.Second,
		Interval:                time.Duration(wire.Interval.Int64()) * time.Second,
	}
	if result.DeviceCode == "" {
		return nil, fmt.Errorf("CliApiKeyAuthorize response missing device code")
	}
	return result, nil
}

func (c *portalClient) CliAPIKeyToken(ctx context.Context, deviceCode string) (*cliAPIKeyTokenResult, error) {
	body, err := c.post(ctx, "CliApiKeyToken", map[string]string{
		"device_code": deviceCode,
	})
	if err != nil {
		return nil, err
	}

	var wire cliAPIKeyTokenWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decoding CliApiKeyToken response: %w", err)
	}

	return &cliAPIKeyTokenResult{
		Status:         wire.Status,
		APIKey:         firstNonEmpty(wire.APIKey, wire.APIKeySnake),
		APIKeyID:       firstNonEmpty(wire.APIKeyID, wire.APIKeyIDSnake),
		APIKeyName:     firstNonEmpty(wire.APIKeyName, wire.APIKeyNameSnake),
		OrganizationID: firstNonEmpty(wire.OrganizationID, wire.OrganizationIDSnake),
	}, nil
}

type portalAPIError struct {
	retryable bool
	err       error
}

func (e *portalAPIError) Error() string   { return e.err.Error() }
func (e *portalAPIError) Unwrap() error   { return e.err }
func (e *portalAPIError) Retryable() bool { return e.retryable }

func (c *portalClient) post(ctx context.Context, method string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, &portalAPIError{err: fmt.Errorf("encoding %s request: %w", method, err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+portalAPIPrefix+method, bytes.NewReader(raw))
	if err != nil {
		return nil, &portalAPIError{err: fmt.Errorf("creating %s request: %w", method, err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", connectProtocolVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &portalAPIError{
			retryable: !errors.Is(err, context.Canceled),
			err:       fmt.Errorf("%s request: %w", method, err),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &portalAPIError{
			retryable: true,
			err:       fmt.Errorf("reading %s response: %w", method, err),
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &portalAPIError{
			retryable: resp.StatusCode >= 500,
			err:       fmt.Errorf("%s failed: %s", method, connectErrorMessage(resp.StatusCode, body)),
		}
	}
	return body, nil
}

type cliAPIKeyAuthorizeWire struct {
	DeviceCode                   string    `json:"deviceCode"`
	DeviceCodeSnake              string    `json:"device_code"`
	UserCode                     string    `json:"userCode"`
	UserCodeSnake                string    `json:"user_code"`
	VerificationURI              string    `json:"verificationUri"`
	VerificationURISnake         string    `json:"verification_uri"`
	VerificationURIComplete      string    `json:"verificationUriComplete"`
	VerificationURICompleteSnake string    `json:"verification_uri_complete"`
	ExpiresIn                    jsonInt64 `json:"expiresIn"`
	Interval                     jsonInt64 `json:"interval"`
}

func (w *cliAPIKeyAuthorizeWire) UnmarshalJSON(data []byte) error {
	type alias cliAPIKeyAuthorizeWire
	var camel alias
	if err := json.Unmarshal(data, &camel); err != nil {
		return err
	}
	*w = cliAPIKeyAuthorizeWire(camel)

	var snake struct {
		ExpiresIn jsonInt64 `json:"expires_in"`
		Interval  jsonInt64 `json:"interval"`
	}
	if err := json.Unmarshal(data, &snake); err != nil {
		return err
	}
	if w.ExpiresIn == 0 {
		w.ExpiresIn = snake.ExpiresIn
	}
	if w.Interval == 0 {
		w.Interval = snake.Interval
	}
	return nil
}

type cliAPIKeyTokenWire struct {
	Status              cliAPIKeyStatus `json:"status"`
	APIKey              string          `json:"apiKey"`
	APIKeySnake         string          `json:"api_key"`
	APIKeyID            string          `json:"apiKeyId"`
	APIKeyIDSnake       string          `json:"api_key_id"`
	APIKeyName          string          `json:"apiKeyName"`
	APIKeyNameSnake     string          `json:"api_key_name"`
	OrganizationID      string          `json:"organizationId"`
	OrganizationIDSnake string          `json:"organization_id"`
}

type jsonInt64 int64

func (j jsonInt64) Int64() int64 { return int64(j) }

func (j *jsonInt64) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*j = jsonInt64(n)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*j = jsonInt64(n)
	return nil
}

func (s *cliAPIKeyStatus) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		*s = parseCLIAPIKeyStatus(raw)
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*s = cliAPIKeyStatusFromNumber(n)
	return nil
}

func parseCLIAPIKeyStatus(raw string) cliAPIKeyStatus {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "PENDING", "CLI_API_KEY_TOKEN_STATUS_PENDING":
		return cliAPIKeyPending
	case "SLOW_DOWN", "CLI_API_KEY_TOKEN_STATUS_SLOW_DOWN":
		return cliAPIKeySlowDown
	case "DENIED", "CLI_API_KEY_TOKEN_STATUS_DENIED":
		return cliAPIKeyDenied
	case "EXPIRED", "CLI_API_KEY_TOKEN_STATUS_EXPIRED":
		return cliAPIKeyExpired
	case "APPROVED", "CLI_API_KEY_TOKEN_STATUS_APPROVED":
		return cliAPIKeyApproved
	default:
		return cliAPIKeyUnknown
	}
}

func cliAPIKeyStatusFromNumber(n int) cliAPIKeyStatus {
	switch n {
	case 1:
		return cliAPIKeyPending
	case 2:
		return cliAPIKeySlowDown
	case 3:
		return cliAPIKeyDenied
	case 4:
		return cliAPIKeyExpired
	case 5:
		return cliAPIKeyApproved
	default:
		return cliAPIKeyUnknown
	}
}

type connectErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func connectErrorMessage(status int, body []byte) string {
	var parsed connectErrorBody
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Message != "" {
		msg := redactSecrets(parsed.Message)
		if parsed.Code != "" {
			return fmt.Sprintf("HTTP %d (%s): %s", status, redactSecrets(parsed.Code), msg)
		}
		return fmt.Sprintf("HTTP %d: %s", status, msg)
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, redactSecrets(trimmed))
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func verificationURL(auth *cliAPIKeyAuthorizeResult) (string, error) {
	raw := auth.VerificationURIComplete
	if raw == "" && auth.VerificationURI != "" && auth.UserCode != "" {
		u, err := url.Parse(auth.VerificationURI)
		if err != nil {
			return "", fmt.Errorf("invalid verification URL: %w", err)
		}
		q := u.Query()
		q.Set("user_code", auth.UserCode)
		u.RawQuery = q.Encode()
		raw = u.String()
	}
	if !isHTTPURL(raw) {
		return "", fmt.Errorf("missing http(s) verification URL")
	}
	return raw, nil
}

func isRetryablePollError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var p *portalAPIError
	return errors.As(err, &p) && p.Retryable()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func runCliAPIKeyAuth(ctx context.Context, client *portalClient, out io.Writer, openURL func(string) error) (string, error) {
	auth, err := client.CliAPIKeyAuthorize(ctx, deviceClientName)
	if err != nil {
		return "", fmt.Errorf("starting login: %w", err)
	}

	verifyURL, err := verificationURL(auth)
	if err != nil {
		return "", fmt.Errorf("starting login: %w", err)
	}

	fmt.Fprintln(out, "Authenticate with The Graph Market.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Open this link, pick an organization if asked, then select an API key:")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "    %s\n", verifyURL)
	fmt.Fprintln(out)
	if auth.UserCode != "" {
		fmt.Fprintf(out, "User code: %s\n", auth.UserCode)
		fmt.Fprintln(out)
	}

	if openURL != nil && verifyURL != "" {
		if err := openURL(verifyURL); err == nil {
			fmt.Fprintln(out, "Opened the key picker in your browser.")
		}
	}

	expiresIn := auth.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 10 * time.Minute
	}
	fmt.Fprintf(out, "Waiting for you to select an API key (expires in %s)...\n", expiresIn.Round(time.Second))

	result, err := waitForCLIAPIKey(ctx, client, auth.DeviceCode, auth.Interval, expiresIn, sleepWithContext)
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out)
	if result.APIKeyName != "" {
		fmt.Fprintf(out, "Selected API key %q.\n", result.APIKeyName)
	} else {
		fmt.Fprintln(out, "Selected an API key.")
	}
	fmt.Fprintln(out)
	return result.APIKey, nil
}

func waitForCLIAPIKey(
	ctx context.Context,
	client cliAPIKeyTokener,
	deviceCode string,
	interval, expiresIn time.Duration,
	sleep func(context.Context, time.Duration) error,
) (*cliAPIKeyTokenResult, error) {
	if sleep == nil {
		sleep = sleepWithContext
	}
	deadline := time.Now().Add(expiresIn)
	wait := interval
	if wait <= 0 {
		wait = defaultDevicePollWait
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("login expired before a key was selected")
		}

		result, err := client.CliAPIKeyToken(ctx, deviceCode)
		if err != nil {
			if !isRetryablePollError(err) {
				return nil, fmt.Errorf("polling login: %w", err)
			}
		} else {
			switch result.Status {
			case cliAPIKeyApproved:
				if result.APIKey == "" {
					return nil, fmt.Errorf("login approved but no API key was returned")
				}
				return result, nil
			case cliAPIKeyDenied:
				return nil, fmt.Errorf("login was denied")
			case cliAPIKeyExpired:
				return nil, fmt.Errorf("login expired before a key was selected")
			case cliAPIKeySlowDown:
				wait += slowDownExtraWait
			}
		}

		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("login expired before a key was selected")
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
