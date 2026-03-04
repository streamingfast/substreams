package client

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/mostynb/go-grpc-compression/experimental/s2"
	"github.com/streamingfast/dgrpc"
	networks "github.com/streamingfast/firehose-networks"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/protodecode"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	xdscreds "google.golang.org/grpc/credentials/xds"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/gzip"
	_ "google.golang.org/grpc/xds"
)

func init() {
	encoding.RegisterCodec(protodecode.PreferredVTCodec{})
}

type AuthType int

const (
	None AuthType = iota
	JWT
	ApiKey
	SecretKey
)

type ProtocolVersion int

const (
	ProtocolVersionUnset ProtocolVersion = 0
	ProtocolVersionV2    ProtocolVersion = 2
	ProtocolVersionV3    ProtocolVersion = 3
	ProtocolVersionV4    ProtocolVersion = 4
)

// String returns the string representation of the protocol version
func (pv ProtocolVersion) String() string {
	switch pv {
	case ProtocolVersionUnset:
		return "unset"
	case ProtocolVersionV2:
		return "v2"
	case ProtocolVersionV3:
		return "v3"
	case ProtocolVersionV4:
		return "v4"
	default:
		return "unknown"
	}
}

// IsValid returns true if the protocol version is valid
func (pv ProtocolVersion) IsValid() bool {
	return pv == ProtocolVersionUnset || pv == ProtocolVersionV2 || pv == ProtocolVersionV3 || pv == ProtocolVersionV4
}

// ParseProtocolVersion parses an integer to a ProtocolVersion
func ParseProtocolVersion(version int) (ProtocolVersion, error) {
	switch version {
	case 0:
		return ProtocolVersionUnset, nil
	case 2:
		return ProtocolVersionV2, nil
	case 3:
		return ProtocolVersionV3, nil
	case 4:
		return ProtocolVersionV4, nil
	default:
		return ProtocolVersionUnset, fmt.Errorf("invalid protocol version %d, only 0, 2, 3 and 4 are supported", version)
	}
}

func (pv ProtocolVersion) IsV2() bool {
	return pv == ProtocolVersionV2
}

func (pv ProtocolVersion) IsV3() bool {
	return pv == ProtocolVersionV3
}

func (pv ProtocolVersion) IsV4() bool {
	return pv == ProtocolVersionV4
}

func (pv ProtocolVersion) IsUnset() bool {
	return pv == ProtocolVersionUnset
}

// SubstreamsClientConfigOptions contains options for creating a new SubstreamsClientConfig
type SubstreamsClientConfigOptions struct {
	// Endpoint is the gRPC endpoint to connect to (e.g., "mainnet.eth.streamingfast.io:443")
	Endpoint string
	// AuthToken is the authentication token (JWT or API key)
	AuthToken string
	// AuthType specifies the type of authentication (None, JWT, ApiKey or Secret)
	AuthType AuthType
	// Insecure allows insecure TLS connections (skips certificate verification)
	Insecure bool
	// PlainText uses unencrypted connections (no TLS)
	PlainText bool
	// Agent is the User-Agent string for gRPC requests
	Agent string
	// ForceProtocolVersion forces the use of a specific protocol version (2, 3 or 4)
	ForceProtocolVersion ProtocolVersion
}

type SubstreamsClientConfig struct {
	endpoint             string
	authToken            string
	authType             AuthType
	insecure             bool
	plaintext            bool
	agent                string
	forceProtocolVersion ProtocolVersion
}

func (c *SubstreamsClientConfig) Agent() string {
	return c.agent
}

// SetAgent sets the User-Agent header for gRPC requests made by this client, this can be
// set at any time but will be effective only before the `NewSubstreamsClient` is called, after
// that the changes to agent will **not** affect already created clients.
func (c *SubstreamsClientConfig) SetAgent(agent string) {
	c.agent = agent
}

func (c *SubstreamsClientConfig) Endpoint() string {
	return c.endpoint
}

func (c *SubstreamsClientConfig) ForceProtocolVersion() ProtocolVersion {
	return c.forceProtocolVersion
}

func (c *SubstreamsClientConfig) SetEndpoint(endpoint string) {
	c.endpoint = endpoint
}

func (c *SubstreamsClientConfig) Insecure() bool {
	return c.insecure
}

func (c *SubstreamsClientConfig) PlainText() bool {
	return c.plaintext
}

func (c *SubstreamsClientConfig) AuthToken() string {
	return c.authToken
}

func (c *SubstreamsClientConfig) AuthType() AuthType {
	return c.authType
}

func (c *SubstreamsClientConfig) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("client_endpoint", c.endpoint)
	encoder.AddBool("client_plaintext", c.plaintext)
	encoder.AddBool("client_insecure", c.insecure)
	encoder.AddBool("jwt_set", c.authToken != "" && c.authType == JWT)
	encoder.AddBool("api_key_set", c.authToken != "" && c.authType == ApiKey)
	encoder.AddBool("secret_set", c.authToken != "" && c.authType == SecretKey)

	return nil
}

type InternalClientFactory = func() (cli pbssinternal.SubstreamsClient, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error)

// NewSubstreamsClientConfig creates a new SubstreamsClientConfig using the provided options.
// This function handles URL parsing for http:// and https:// schemes, automatically setting
// appropriate plaintext and port defaults.
//
// Example usage:
//
//	config := client.NewSubstreamsClientConfig(client.NewSubstreamsClientConfigOptions{
//	    Endpoint:  "mainnet.eth.streamingfast.io:443",
//	    AuthToken: "your-auth-token",
//	    AuthType:  client.JWT,
//	    Agent:     "my-application",
//	})
func NewSubstreamsClientConfig(opts SubstreamsClientConfigOptions) *SubstreamsClientConfig {
	endpoint := opts.Endpoint
	plaintext := opts.PlainText

	// Check for http:// or https:// prefix and adjust settings accordingly
	if len(endpoint) > 7 && endpoint[:7] == "http://" {
		plaintext = true
		parsedURL, err := url.Parse(endpoint)
		if err == nil && parsedURL.Port() == "" {
			// No port specified, append default port for HTTP
			endpoint = parsedURL.Host + ":80"
		} else if err == nil {
			// Port is already specified or there was an error, just strip the scheme
			endpoint = parsedURL.Host
		} else {
			// Fallback to simple stripping if parsing fails
			endpoint = endpoint[7:]
		}
	} else if len(endpoint) > 8 && endpoint[:8] == "https://" {
		plaintext = false
		parsedURL, err := url.Parse(endpoint)
		if err == nil && parsedURL.Port() == "" {
			// No port specified, append default port for HTTPS
			endpoint = parsedURL.Host + ":443"
		} else if err == nil {
			// Port is already specified or there was an error, just strip the scheme
			endpoint = parsedURL.Host
		} else {
			// Fallback to simple stripping if parsing fails
			endpoint = endpoint[8:]
		}
	} else {
		if resolvedEndpoint := networks.GetSubstreamsEndpoint(endpoint); resolvedEndpoint != "" {
			// Resolved endpoint from alias is always using non-plaintext
			endpoint = resolvedEndpoint
			plaintext = false
		}
	}

	return &SubstreamsClientConfig{
		endpoint:             endpoint,
		authToken:            opts.AuthToken,
		authType:             opts.AuthType,
		insecure:             opts.Insecure,
		plaintext:            plaintext,
		agent:                opts.Agent,
		forceProtocolVersion: opts.ForceProtocolVersion,
	}
}

var portSuffixRegex = regexp.MustCompile(":[0-9]{2,5}$")

func NewInternalClientFactory(config *SubstreamsClientConfig) InternalClientFactory {
	bootStrapFilename := os.Getenv("GRPC_XDS_BOOTSTRAP")

	if bootStrapFilename == "" {
		zlog.Info("setting up basic grpc client factory (no XDS bootstrap)")

		return func() (cli pbssinternal.SubstreamsClient, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error) {
			return NewSubstreamsInternalClient(config)
		}
	}

	zlog.Info("setting up xds grpc client factory", zap.String("GRPC_XDS_BOOTSTRAP", bootStrapFilename))

	noop := func() error { return nil }
	cli, _, callOpts, headers, err := NewSubstreamsInternalClient(config)
	if err != nil {
		zlog.Error("failed to create substreams client", zap.Error(err))
	}
	return func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, Headers, error) {
		return cli, noop, callOpts, headers, err
	}
}

func NewSubstreamsInternalClient(config *SubstreamsClientConfig) (cli pbssinternal.SubstreamsClient, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error) {
	if config == nil {
		return nil, nil, nil, nil, fmt.Errorf("substreams client config not set")
	}
	endpoint := config.endpoint
	authToken := config.authToken
	authType := config.authType
	usePlainTextConnection := config.plaintext
	useInsecureTLSConnection := config.insecure

	if !portSuffixRegex.MatchString(endpoint) {
		return nil, nil, nil, nil, fmt.Errorf("invalid endpoint %q: endpoint's suffix must be a valid port in the form ':<port>', port 443 is usually the right one to use", endpoint)
	}

	bootStrapFilename := os.Getenv("GRPC_XDS_BOOTSTRAP")

	var dialOptions []grpc.DialOption
	if bootStrapFilename != "" {
		log.Println("Using xDS credentials...")
		creds, err := xdscreds.NewClientCredentials(xdscreds.ClientOptions{FallbackCreds: insecure.NewCredentials()})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to create xDS credentials: %v", err)
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else {
		switch {
		case usePlainTextConnection:
			zlog.Debug("setting plain text option")
			dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

		case useInsecureTLSConnection:
			zlog.Debug("setting insecure tls connection option")
			dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))}
		}
	}

	if messageLimit, ok := getGRPCSizeLoggerFromEnv(); ok {
		sizeHandler := dgrpc.NewSizeLoggingHandler(messageLimit, zlog)
		dialOptions = append(dialOptions, grpc.WithStatsHandler(sizeHandler))
	}

	zlog.Debug("getting connection", zap.String("endpoint", endpoint))
	conn, err := dgrpc.NewExternalClientConn(endpoint, dialOptions...)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("unable to create external gRPC client: %w", err)
	}
	closeFunc = conn.Close

	switch authType {
	case JWT:
		zlog.Debug("creating oauth access", zap.String("endpoint", endpoint))
		tokenSource := oauth.TokenSource{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: authToken, TokenType: "Bearer"})}
		callOpts = append(callOpts, grpc.PerRPCCredentials(tokenSource))
	case ApiKey:
		zlog.Debug("creating api key access", zap.String("endpoint", endpoint))
		headers = map[string]string{ApiKeyHeader: authToken}
	case SecretKey:
		zlog.Debug("using secret key", zap.String("endpoint", endpoint))
		headers = map[string]string{SecretKeyHeader: authToken}
	}

	zlog.Debug("creating new client", zap.String("endpoint", endpoint))
	cli = pbssinternal.NewSubstreamsClient(conn)
	zlog.Debug("client created")
	return
}

func newConnection(config *SubstreamsClientConfig) (conn *grpc.ClientConn, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error) {
	if config == nil {
		return nil, nil, nil, nil, fmt.Errorf("substreams client config not set")
	}
	endpoint := config.endpoint
	authToken := config.authToken
	authType := config.authType
	usePlainTextConnection := config.plaintext
	useInsecureTLSConnection := config.insecure

	zlog.Debug("creating new client", zap.String("endpoint", endpoint))

	if !portSuffixRegex.MatchString(endpoint) {
		return nil, nil, nil, nil, fmt.Errorf("invalid endpoint %q: endpoint's suffix must be a valid port in the form ':<port>', port 443 is usually the right one to use", endpoint)
	}

	bootStrapFilename := os.Getenv("GRPC_XDS_BOOTSTRAP")

	var dialOptions []grpc.DialOption
	skipAuth := authType == None || usePlainTextConnection
	if bootStrapFilename != "" {
		log.Println("Using xDS credentials...")
		creds, err := xdscreds.NewClientCredentials(xdscreds.ClientOptions{FallbackCreds: insecure.NewCredentials()})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to create xDS credentials: %v", err)
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else {
		if useInsecureTLSConnection && usePlainTextConnection {
			return nil, nil, nil, nil, fmt.Errorf("option --insecure and --plaintext are mutually exclusive, they cannot be both specified at the same time")
		}
		switch {
		case usePlainTextConnection:
			zlog.Debug("setting plain text option")

			dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

		case useInsecureTLSConnection:
			zlog.Debug("setting insecure tls connection option")
			dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))}
		}
	}

	if messageLimit, ok := getGRPCSizeLoggerFromEnv(); ok {
		sizeHandler := dgrpc.NewSizeLoggingHandler(messageLimit, zlog)
		dialOptions = append(dialOptions, grpc.WithStatsHandler(sizeHandler))
	}

	dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(grpc.UseCompressor(s2.Name)))
	dialOptions = append(dialOptions, grpc.WithUserAgent(config.agent))

	zlog.Debug("getting connection", zap.String("endpoint", endpoint))
	conn, err = dgrpc.NewExternalClient(endpoint, dialOptions...)

	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("unable to create external gRPC client: %w", err)
	}
	closeFunc = conn.Close

	if !skipAuth {
		if authType == JWT {
			zlog.Debug("creating oauth access", zap.String("endpoint", endpoint))
			tokenSource := oauth.TokenSource{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: authToken, TokenType: "Bearer"})}
			callOpts = append(callOpts, grpc.PerRPCCredentials(tokenSource))
		} else if authType == ApiKey {
			zlog.Debug("creating api key access", zap.String("endpoint", endpoint))
			headers = map[string]string{ApiKeyHeader: authToken}
		}
	}

	return
}

func NewSubstreamsClientConn(config *SubstreamsClientConfig) (conn *grpc.ClientConn, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error) {
	return newConnection(config)
}

func getGRPCSizeLoggerFromEnv() (limit int, ok bool) {
	messageLimitString := os.Getenv("GRPC_SIZE_LOGGER_MESSAGE_LIMIT")
	if messageLimitString == "" {
		return 0, false
	}
	messageLimit := 10000
	var err error
	if messageLimitString != "" {
		messageLimit, err = strconv.Atoi(messageLimitString)
		if err != nil {
			zlog.Warn("failed to parse GRPC_SIZE_LOGGER_MESSAGE_LIMIT", zap.Error(err))
			return 0, false
		}
	}
	return messageLimit, true
}
