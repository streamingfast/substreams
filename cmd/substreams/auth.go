package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
)

const (
	defaultPortalAPIURL = "https://admin.streamingfast.io"
	localPortalAPIURL   = "http://localhost:9000"
	defaultMarketURL    = "https://thegraph.market"
	localMarketURL      = "http://localhost:3000"
	deviceClientName    = "substreams CLI"
	authEnvFilename     = ".substreams.env"
	// defaultAPIKeyEnvvar is the default of run's --api-key-envvar.
	defaultAPIKeyEnvvar = "SUBSTREAMS_API_KEY"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Login for Substreams development",
	Long: `Login to The Graph Market and retrieve an API key without copy/paste.

The command opens a browser so you can pick an organization (if you have more
than one) and an API key. The selected key is fetched over the API and written
to .substreams.env as SUBSTREAMS_API_KEY.

Use --paste to enter an API key yourself.`,
	RunE:         runAuthE,
	SilenceUsage: true,
}

func init() {
	authCmd.Flags().Bool("paste", false, "Paste an API key instead of picking one in the browser")
	rootCmd.AddCommand(authCmd)
}

func runAuthE(cmd *cobra.Command, args []string) error {
	if sflags.MustGetBool(cmd, "paste") {
		apiKey, err := runPasteAuth()
		if err != nil {
			return err
		}
		return writeAuthCredentialsAndHint(apiKey)
	}

	key, err := authenticateInteractively(cmd.Context(), os.Stdout)
	if err != nil {
		return err
	}

	return writeAuthCredentialsAndHint(key)
}

func writeAuthCredentialsAndHint(apiKey string) error {
	if err := writeAuthCredentials(os.Stdout, defaultAPIKeyEnvvar, apiKey); err != nil {
		return err
	}

	fmt.Println("Load credentials in current terminal with the following command:")
	fmt.Println()
	fmt.Println(cli.PurpleStyle.Render("       . ./.substreams.env"))
	fmt.Println()
	return nil
}

// authenticateInteractively runs the browser key picker and returns the API key
// selected there.
func authenticateInteractively(ctx context.Context, out io.Writer) (string, error) {
	key, err := runCliAPIKeyAuth(ctx, newPortalClient(portalAPIBaseURL()), out, tryOpenURL)
	if errors.Is(err, context.Canceled) {
		return "", fmt.Errorf("login cancelled")
	}
	return key, err
}

func runPasteAuth() (string, error) {
	baseURL := defaultMarketURL
	if os.Getenv("LOCAL_DEVELOPMENT") == "true" {
		baseURL = localMarketURL
	}

	signupURL := baseURL + "/auth/signup"

	fmt.Println("Authenticate with The Graph Market to access Substreams endpoints.")
	fmt.Println()
	fmt.Println("If you don't have an account yet, register and create an API key here:")
	fmt.Println("    " + cli.PurpleStyle.Render(signupURL))
	fmt.Println()

	var apiKey string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title("Paste your API key here:").
				Inline(true).
				Value(&apiKey).
				Validate(validateAPIKey),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("error running form: %w", err)
	}

	return apiKey, nil
}

// validateAPIKey rejects empty input and JWTs, which are not API keys and would be
// rejected by the server when sent as the `x-api-key` header.
func validateAPIKey(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("API key cannot be empty")
	}
	if strings.HasPrefix(value, "eyJ") {
		return errors.New("this looks like a JWT, not an API key; to use a JWT, export SUBSTREAMS_API_TOKEN yourself")
	}
	return nil
}

// writeAuthCredentials writes apiKey to ./.substreams.env under envvar, the
// variable the run reading it looks the key up in.
func writeAuthCredentials(out io.Writer, envvar string, apiKey string) error {
	if _, err := os.Stat(authEnvFilename); err == nil {
		fmt.Fprintln(out, "Replacing the existing `./.substreams.env`.  NOTE: Add it to `.gitignore`.")
	} else {
		fmt.Fprintln(out, "Writing `./.substreams.env`.  NOTE: Add it to `.gitignore`.")
	}
	fmt.Fprintln(out)

	if err := writeAuthEnvFile(authEnvFilename, envvar, apiKey); err != nil {
		return fmt.Errorf("writing .substreams.env file: %w", err)
	}
	return nil
}

func writeAuthEnvFile(path string, envvar string, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("no API key to write")
	}
	content := fmt.Sprintf("export %s=%s\n", envvar, apiKey)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	// WriteFile only applies the mode on create; re-login must tighten an existing file.
	return os.Chmod(path, 0o600)
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

func tryOpenURL(rawURL string) error {
	if !isHTTPURL(rawURL) {
		return fmt.Errorf("refusing to open non-http(s) URL")
	}
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

var (
	serverKeyPattern       = regexp.MustCompile(`server_[A-Za-z0-9]+`)
	apiKeyJSONPattern      = regexp.MustCompile(`"api_key"\s*:\s*"[^"]*"`)
	deviceCodeJSONPattern  = regexp.MustCompile(`"device_code"\s*:\s*"[^"]*"`)
	deviceCodeCamelPattern = regexp.MustCompile(`"deviceCode"\s*:\s*"[^"]*"`)
)

func redactSecrets(s string) string {
	s = apiKeyJSONPattern.ReplaceAllString(s, `"api_key":"[redacted]"`)
	s = deviceCodeJSONPattern.ReplaceAllString(s, `"device_code":"[redacted]"`)
	s = deviceCodeCamelPattern.ReplaceAllString(s, `"deviceCode":"[redacted]"`)
	return serverKeyPattern.ReplaceAllString(s, "server_[redacted]")
}
