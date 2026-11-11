package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/sink"
)

// sinkerConfigWithAuthPrompt builds the config of run or gui, offering to
// authenticate first when it would reach a StreamingFast endpoint without
// credentials, and building it again with the selected key.
func sinkerConfigWithAuthPrompt(cmd *cobra.Command, build func() (*sink.SinkerConfig, error)) (*sink.SinkerConfig, error) {
	config, err := build()
	if err != nil {
		return nil, err
	}

	authenticated, err := defaultAuthPrompter().maybeAuthenticate(cmd.Context(), config.ClientConfig, sflags.MustGetString(cmd, sink.FlagAPIKeyEnvvar))
	if err != nil || !authenticated {
		return config, err
	}
	return build()
}

// authPrompter offers the `substreams auth` flow before a run that would
// otherwise end in an Unauthenticated error.
type authPrompter struct {
	interactive      func() bool
	confirm          func(question string) (bool, error)
	authenticate     func(ctx context.Context, out io.Writer) (string, error)
	writeCredentials func(out io.Writer, envvar string, apiKey string) error
	setenv           func(key, value string) error
	out              io.Writer
}

func defaultAuthPrompter() authPrompter {
	return authPrompter{
		interactive: func() bool {
			return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stderr.Fd())
		},
		confirm:          confirmOnStderr,
		authenticate:     authenticateInteractively,
		writeCredentials: writeAuthCredentials,
		setenv:           os.Setenv,
		out:              os.Stderr,
	}
}

// maybeAuthenticate asks to authenticate when cfg has no credentials for a
// StreamingFast endpoint and someone is at the terminal to answer. On yes it
// runs the `substreams auth` flow, writes .substreams.env and sets apiKeyEnvvar
// for this process, then reports true.
func (p authPrompter) maybeAuthenticate(ctx context.Context, cfg *client.SubstreamsClientConfig, apiKeyEnvvar string) (bool, error) {
	if !needsAuthPrompt(cfg) || !p.interactive() {
		return false, nil
	}

	yes, err := p.confirm("No Substreams credentials found. Authenticate now with The Graph Market?")
	if err != nil || !yes {
		return false, err
	}

	apiKey, err := p.authenticate(ctx, p.out)
	if err != nil {
		return false, err
	}
	if err := p.writeCredentials(p.out, apiKeyEnvvar, apiKey); err != nil {
		return false, err
	}
	if err := p.setenv(apiKeyEnvvar, strings.TrimSpace(apiKey)); err != nil {
		return false, fmt.Errorf("setting %s: %w", apiKeyEnvvar, err)
	}
	return true, nil
}

// needsAuthPrompt is true for a StreamingFast endpoint reached with no
// credentials. Plaintext and other endpoints may well need none.
func needsAuthPrompt(cfg *client.SubstreamsClientConfig) bool {
	return cfg != nil && cfg.AuthType() == client.None && !cfg.PlainText() && isStreamingFastEndpoint(cfg.Endpoint())
}

func isStreamingFastEndpoint(endpoint string) bool {
	host := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(endpoint)), "https://")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == "streamingfast.io" || strings.HasSuffix(host, ".streamingfast.io")
}

func confirmOnStderr(question string) (bool, error) {
	yes := true
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&yes),
	)).WithTheme(huhTheme).WithAccessible(WITH_ACCESSIBLE).WithOutput(os.Stderr).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return false, fmt.Errorf("cancelled")
	}
	if err != nil {
		return false, fmt.Errorf("failed taking confirmation input: %w", err)
	}
	return yes, nil
}
