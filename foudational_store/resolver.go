package foudational_store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/streamingfast/dregistry"
	"github.com/streamingfast/dregistry/plugins"
	"go.uber.org/zap"
)

// Plugin registration lives here so this module can resolve stores without a
// firehose-core wrapper. The same Registers call can move to firehose-core
// later, the way dauth plugins are blank-imported there.
var registerPlugins = sync.OnceValue(func() error {
	return plugins.Registers("json", "controlplane", "passthrough")
})

// NewResolver builds the 3-tier foundational-store resolver used by tier1:
// JSON map (if any) → control-plane (if address set, with @v strip) → identifier passthrough.
func NewResolver(endpoints map[string]string, registryAddress string, logger *zap.Logger) (dregistry.Resolver, error) {
	if err := registerPlugins(); err != nil {
		return nil, err
	}

	var resolvers []dregistry.Resolver
	if len(endpoints) > 0 {
		jsonResolver, err := jsonResolverFromMap(endpoints)
		if err != nil {
			return nil, err
		}
		resolvers = append(resolvers, jsonResolver)
	}
	if registryAddress != "" {
		controlPlane, err := dregistry.New(controlPlaneDSN(registryAddress), logger)
		if err != nil {
			return nil, fmt.Errorf("control-plane registry resolver: %w", err)
		}
		resolvers = append(resolvers, stripDeploymentID{inner: controlPlane})
	}
	passthrough, err := NewPassthroughResolver()
	if err != nil {
		return nil, err
	}
	resolvers = append(resolvers, passthrough)
	if len(resolvers) == 1 {
		return resolvers[0], nil
	}
	return dregistry.Chain(resolvers...), nil
}

// NewStaticResolver looks up identifiers in a pre-resolved map. Tier2 uses this
// because tier1 already resolved every identifier onto the subrequest.
func NewStaticResolver(endpoints map[string]string) (dregistry.Resolver, error) {
	if len(endpoints) == 0 {
		return staticResolver{}, nil
	}
	if err := registerPlugins(); err != nil {
		return nil, err
	}
	return jsonResolverFromMap(endpoints)
}

// NewPassthroughResolver treats the identifier itself as the endpoint.
func NewPassthroughResolver() (dregistry.Resolver, error) {
	if err := registerPlugins(); err != nil {
		return nil, err
	}
	return dregistry.New("passthrough://", nil)
}

type staticResolver struct{}

func (staticResolver) Resolve(_ context.Context, identifier string) (*dregistry.Endpoint, error) {
	return nil, fmt.Errorf("%w: %q", dregistry.ErrNotFound, identifier)
}

// PrependJSON puts a static identifier→endpoint map in front of an existing resolver.
func PrependJSON(endpoints map[string]string, next dregistry.Resolver) (dregistry.Resolver, error) {
	if len(endpoints) == 0 {
		return next, nil
	}
	if err := registerPlugins(); err != nil {
		return nil, err
	}
	jsonResolver, err := jsonResolverFromMap(endpoints)
	if err != nil {
		return nil, err
	}
	if next == nil {
		return jsonResolver, nil
	}
	return dregistry.Chain(jsonResolver, next), nil
}

func jsonResolverFromMap(endpoints map[string]string) (dregistry.Resolver, error) {
	payload, err := json.Marshal(endpoints)
	if err != nil {
		return nil, fmt.Errorf("marshal foundational store endpoints: %w", err)
	}
	resolver, err := dregistry.New("json://"+string(payload), nil)
	if err != nil {
		return nil, fmt.Errorf("json registry resolver: %w", err)
	}
	return resolver, nil
}

func controlPlaneDSN(addr string) string {
	// dregistry names the plaintext override `insecure` (it maps onto
	// dgrpc.WithAutoTransportCredentials(..., plainText, ...)). The old
	// HostedStoreRegistryAddress dial was always plaintext.
	switch {
	case strings.HasPrefix(addr, "controlplane://"):
		return addr
	case strings.HasPrefix(addr, "grpcs://"):
		return withInsecureQuery("controlplane://"+strings.TrimPrefix(addr, "grpcs://"), false)
	case strings.HasPrefix(addr, "grpc://"):
		return withInsecureQuery("controlplane://"+strings.TrimPrefix(addr, "grpc://"), true)
	default:
		return withInsecureQuery("controlplane://"+addr, true)
	}
}

func withInsecureQuery(dsn string, insecure bool) string {
	if strings.Contains(dsn, "insecure=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	if insecure {
		return dsn + sep + "insecure=true"
	}
	return dsn + sep + "insecure=false"
}

// EncodeEndpoint turns a resolved endpoint into the string we send to tier2.
// The scheme carries TLS so the json resolver on the other side can recover it.
func EncodeEndpoint(endpoint *dregistry.Endpoint) string {
	if endpoint == nil || endpoint.Address == "" {
		return ""
	}
	if endpoint.TLS {
		return "grpcs://" + endpoint.Address
	}
	return "grpc://" + endpoint.Address
}

// DecodeEndpoint is the inverse of [EncodeEndpoint].
func DecodeEndpoint(raw string) (*dregistry.Endpoint, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty endpoint", dregistry.ErrNotFound)
	}
	useTLS := false
	address := raw
	switch {
	case strings.HasPrefix(raw, "grpcs://"):
		useTLS = true
		address = strings.TrimPrefix(raw, "grpcs://")
	case strings.HasPrefix(raw, "grpc://"):
		address = strings.TrimPrefix(raw, "grpc://")
	default:
		useTLS = strings.HasSuffix(raw, ":443")
	}
	return &dregistry.Endpoint{Address: address, TLS: useTLS}, nil
}

// Lookup returns the pre-resolved endpoint for identifier from a map that
// [ResolveIdentifiers] (or the ProcessRange proto field) already filled.
func Lookup(endpoints map[string]string, identifier string) (*dregistry.Endpoint, error) {
	raw, ok := endpoints[identifier]
	if !ok {
		return nil, fmt.Errorf("%w: %q", dregistry.ErrNotFound, identifier)
	}
	return DecodeEndpoint(raw)
}

// ResolveIdentifiers looks up each identifier once. The caller (tier1) ships the
// resulting map to tier2 so workers do not dial the control plane themselves.
func ResolveIdentifiers(ctx context.Context, resolver dregistry.Resolver, identifiers []string) (map[string]string, error) {
	if resolver == nil {
		return nil, fmt.Errorf("foundational store resolver is required")
	}
	out := make(map[string]string, len(identifiers))
	for _, identifier := range identifiers {
		endpoint, err := resolver.Resolve(ctx, identifier)
		if err != nil {
			return nil, fmt.Errorf("resolve foundational store %q: %w", identifier, err)
		}
		out[identifier] = EncodeEndpoint(endpoint)
	}
	return out, nil
}

// DeploymentID strips the trailing "@v<version>" suffix from a hosted-store identifier.
func DeploymentID(identifier string) string {
	if idx := strings.LastIndex(identifier, "@v"); idx > 0 {
		return identifier[:idx]
	}
	return identifier
}

type stripDeploymentID struct {
	inner dregistry.Resolver
}

func (s stripDeploymentID) Resolve(ctx context.Context, identifier string) (*dregistry.Endpoint, error) {
	return s.inner.Resolve(ctx, DeploymentID(identifier))
}
