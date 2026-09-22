package squash

import (
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

type FactoryFunc func(config string, logger *zap.Logger) (Client, error)

var registry = map[string]FactoryFunc{}

func init() {
	Register("local", func(string, *zap.Logger) (Client, error) {
		return nil, nil
	})
}

// Register adds a squasher plugin under the given URL scheme, the same way
// dauth plugins register as "null", "trust", "grpc", "secret".
func Register(name string, factory FactoryFunc) {
	registry[name] = factory
}

// New constructs a squasher Client from a plugin DSN.
// Empty or local:// keeps in-process squashing (a nil Client).
func New(config string, logger *zap.Logger) (Client, error) {
	if config == "" {
		return nil, nil
	}
	u, err := url.Parse(config)
	if err != nil {
		return nil, fmt.Errorf("parse squasher plugin %q: %w", config, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("squasher plugin %q has no scheme", config)
	}
	factory := registry[u.Scheme]
	if factory == nil {
		return nil, fmt.Errorf("no Squasher plugin named %q is currently registered", u.Scheme)
	}
	return factory(config, logger)
}
