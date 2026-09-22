package grpc

import (
	"fmt"
	"net"
	"net/url"

	"github.com/streamingfast/substreams/squash"
	"go.uber.org/zap"
)

// Register adds the grpc:// and grpcs:// squasher plugins to the substreams
// squash registry, the same way dauth/grpc.Register adds grpc:// for auth.
func Register() {
	factory := func(configURL string, logger *zap.Logger) (squash.Client, error) {
		return newClientFromURL(configURL, logger)
	}
	squash.Register("grpc", factory)
	squash.Register("grpcs", factory)
}

func newClientFromURL(configURL string, logger *zap.Logger) (squash.Client, error) {
	endpoint, opts, err := parsePluginURL(configURL)
	if err != nil {
		return nil, err
	}

	logger.Info("setting up grpc squasher",
		zap.String("endpoint", endpoint),
		zap.Bool("plaintext", opts.PlainText),
		zap.Bool("insecure", opts.Insecure),
	)

	client, _, err := Dial(endpoint, opts)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func parsePluginURL(configURL string) (string, DialOptions, error) {
	u, err := url.Parse(configURL)
	if err != nil {
		return "", DialOptions{}, fmt.Errorf("parse squasher plugin: %w", err)
	}
	if u.Hostname() == "" {
		return "", DialOptions{}, fmt.Errorf("squasher plugin %q is missing host", configURL)
	}

	q := u.Query()
	opts := DialOptions{
		Insecure: q.Get("insecure") == "true",
		Secret:   q.Get("secret"),
	}

	host := u.Host
	switch u.Scheme {
	case "grpcs":
		opts.PlainText = false
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "443")
		}
	default:
		opts.PlainText = q.Get("plaintext") != "false"
		if u.Port() == "" {
			return "", DialOptions{}, fmt.Errorf("squasher plugin %q is missing port (grpcs:// defaults to 443)", configURL)
		}
	}
	return host, opts, nil
}
