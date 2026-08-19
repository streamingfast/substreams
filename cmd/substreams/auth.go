package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
)

const (
	defaultPortalAPIURL = "https://admin.streamingfast.io"
	localPortalAPIURL   = "http://localhost:9000"
	defaultAuthBaseURL  = "https://auth.thegraph.market"
	localAuthBaseURL    = "http://localhost:8080"
	defaultMarketURL    = "https://thegraph.market"
	localMarketURL      = "http://localhost:3000"
	deviceClientName    = "substreams CLI"
	authEnvFilename     = ".substreams.env"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Login for Substreams development",
	Long: `Login to The Graph Market and retrieve an API key without copy/paste.

The command opens a browser so you can pick an organization (if you have more
than one) and an API key. The selected key is fetched over the API, exchanged
for a JWT, and written to .substreams.env.

Use --paste to enter a JWT or API key yourself.`,
	RunE:         runAuthE,
	SilenceUsage: true,
}

func init() {
	authCmd.Flags().Bool("paste", false, "Paste a JWT or API key instead of picking one in the browser")
	rootCmd.AddCommand(authCmd)
}

func runAuthE(cmd *cobra.Command, args []string) error {
	if sflags.MustGetBool(cmd, "paste") {
		creds, err := runPasteAuth()
		if err != nil {
			return err
		}
		return writeAuthCredentials(creds)
	}

	key, err := runCliAPIKeyAuth(cmd.Context(), newPortalClient(portalAPIBaseURL()), os.Stdout, tryOpenURL)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("login cancelled")
		}
		return err
	}

	creds, err := resolveCredentials(key)
	if err != nil {
		return err
	}
	return writeAuthCredentials(creds)
}

func runPasteAuth() (authCredentials, error) {
	baseURL := defaultMarketURL
	if os.Getenv("LOCAL_DEVELOPMENT") == "true" {
		baseURL = localMarketURL
	}

	signupURL := baseURL + "/auth/signup"

	fmt.Println("Authenticate with The Graph Market to access Substreams endpoints.")
	fmt.Println()
	fmt.Println("If you don't have an account yet, register and paste back your API key here:")
	fmt.Println("    " + cli.PurpleStyle.Render(signupURL))
	fmt.Println()
	fmt.Println("If you already have an account, follow this link to generate a JWT token:")
	fmt.Println("    " + cli.PurpleStyle.Render(baseURL+"/auth/substreams-devenv"))
	fmt.Println()

	var token string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title("Paste your JWT token or API key here:").
				Inline(true).
				Value(&token).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("token cannot be empty")
					}
					return nil
				}),
		),
	)

	if err := form.Run(); err != nil {
		return authCredentials{}, fmt.Errorf("error running form: %w", err)
	}

	return resolveCredentials(token)
}

type authCredentials struct {
	token  string
	apiKey string
}

func resolveCredentials(value string) (authCredentials, error) {
	if !strings.HasPrefix(value, "server_") {
		return authCredentials{token: value}, nil
	}

	fmt.Println("Exchanging API key for JWT token...")
	jwtToken, err := exchangeAPIKeyForJWT(value, authIssueBaseURL())
	if err != nil {
		if os.Getenv("LOCAL_DEVELOPMENT") == "true" {
			fmt.Println("Could not issue a JWT from the local issuer; writing the API key instead.")
			fmt.Println()
			return authCredentials{apiKey: value}, nil
		}
		return authCredentials{}, fmt.Errorf("exchanging API key for JWT token: %w", err)
	}
	fmt.Println("Successfully obtained JWT token.")
	fmt.Println()
	return authCredentials{token: jwtToken}, nil
}

func writeAuthCredentials(creds authCredentials) error {
	fmt.Println("Writing `./.substreams.env`.  NOTE: Add it to `.gitignore`.")
	fmt.Println()

	if err := writeAuthEnvFile(authEnvFilename, creds); err != nil {
		return fmt.Errorf("writing .substreams.env file: %w", err)
	}

	fmt.Println("Load credentials in current terminal with the following command:")
	fmt.Println()
	fmt.Println(cli.PurpleStyle.Render("       . ./.substreams.env"))
	fmt.Println()

	return nil
}

func writeAuthEnvFile(path string, creds authCredentials) error {
	if creds.token == "" && creds.apiKey == "" {
		return errors.New("no credentials to write")
	}
	var b strings.Builder
	if creds.token != "" {
		fmt.Fprintf(&b, "export SUBSTREAMS_API_TOKEN=%s\n", creds.token)
	}
	if creds.apiKey != "" {
		fmt.Fprintf(&b, "export SUBSTREAMS_API_KEY=%s\n", creds.apiKey)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func portalAPIBaseURL() string {
	if u := strings.TrimSpace(os.Getenv("SUBSTREAMS_PORTAL_API")); u != "" {
		return strings.TrimRight(u, "/")
	}
	if os.Getenv("LOCAL_DEVELOPMENT") == "true" {
		return localPortalAPIURL
	}
	return defaultPortalAPIURL
}

func authIssueBaseURL() string {
	if u := strings.TrimSpace(os.Getenv("SUBSTREAMS_AUTH_ISSUE_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	if os.Getenv("LOCAL_DEVELOPMENT") == "true" {
		return localAuthBaseURL
	}
	return defaultAuthBaseURL
}

func tryOpenURL(rawURL string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{rawURL}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		name, args = "xdg-open", []string{rawURL}
	}
	return exec.Command(name, args...).Start()
}

func exchangeAPIKeyForJWT(apiKey string, authBaseURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := fmt.Sprintf(`{"api_key":"%s"}`, apiKey)
	body := strings.NewReader(data)

	request, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/v1/auth/issue", authBaseURL), body)
	if err != nil {
		return "", fmt.Errorf("creating HTTP request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("HTTP POST request: %w", err)
	}
	defer response.Body.Close()

	fullBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if response.StatusCode != 200 {
		return "", fmt.Errorf("request failed with code %d: %s", response.StatusCode, redactSecrets(string(fullBody)))
	}

	var authResp authIssueResponse
	if err := json.Unmarshal(fullBody, &authResp); err != nil {
		return "", fmt.Errorf("unmarshalling response %q: %w", redactSecrets(string(fullBody)), err)
	}

	return authResp.Token, nil
}

type authIssueResponse struct {
	Token string `json:"token"`
}

var (
	serverKeyPattern  = regexp.MustCompile(`server_[A-Za-z0-9]+`)
	apiKeyJSONPattern = regexp.MustCompile(`"api_key"\s*:\s*"[^"]*"`)
)

func redactSecrets(s string) string {
	s = apiKeyJSONPattern.ReplaceAllString(s, `"api_key":"[redacted]"`)
	return serverKeyPattern.ReplaceAllString(s, "server_[redacted]")
}
