package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
)

var authCmd = &cobra.Command{
	Use:          "auth",
	Short:        "Login command for Substreams development",
	RunE:         runAuthE,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func runAuthE(cmd *cobra.Command, args []string) error {
	localDevelopment := os.Getenv("LOCAL_DEVELOPMENT")

	baseURL := "https://thegraph.market"
	authBaseURL := "https://auth.thegraph.market"

	if localDevelopment == "true" {
		baseURL = "http://localhost:3000"
	}

	signupURL := baseURL + "/auth/signup"

	fmt.Println("Authenticate with The Graph Market to access Substreams endpoints.")
	fmt.Println()
	fmt.Println("If you don't have an account yet, register and paste back your API key here:")
	fmt.Println("    " + cli.PurpleStyle.Render(signupURL))
	fmt.Println()
	fmt.Println("If you already have an account, follow this link to generate a JWT token:")
	fmt.Println("    " + cli.PurpleStyle.Render(baseURL+"/auth/substreams-devenv"))
	fmt.Println("")

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
		return fmt.Errorf("error running form: %w", err)
	}

	if strings.HasPrefix(token, "server_") {
		fmt.Println("Detected API key, exchanging for JWT token...")
		jwtToken, err := exchangeAPIKeyForJWT(token, authBaseURL)
		if err != nil {
			return fmt.Errorf("exchanging API key for JWT token: %w", err)
		}
		token = jwtToken
		fmt.Println("Successfully obtained JWT token.")
		fmt.Println()
	}

	fmt.Println("Writing `./.substreams.env`.  NOTE: Add it to `.gitignore`.")
	fmt.Println("")

	err := os.WriteFile(".substreams.env", []byte(fmt.Sprintf("export SUBSTREAMS_API_TOKEN=%s\n", token)), 0644)
	if err != nil {
		return fmt.Errorf("writing .substreams.env file: %w", err)
	}

	fmt.Println("Load credentials in current terminal with the following command:")
	fmt.Println("")
	fmt.Println(cli.PurpleStyle.Render("       . ./.substreams.env"))
	fmt.Println()

	return nil
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
		return "", fmt.Errorf("request failed with code %d: %s", response.StatusCode, fullBody)
	}

	var authResp authIssueResponse
	if err := json.Unmarshal(fullBody, &authResp); err != nil {
		return "", fmt.Errorf("unmarshalling response %q: %w", fullBody, err)
	}

	return authResp.Token, nil
}

type authIssueResponse struct {
	Token string `json:"token"`
}
