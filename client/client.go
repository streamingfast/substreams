package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mostynb/go-grpc-compression/experimental/s2"

	//"github.com/mostynb/go-grpc-compression/experimental/s2"
	//_ "github.com/mostynb/go-grpc-compression/experimental/s2"
	"github.com/streamingfast/dgrpc"
	networks "github.com/streamingfast/firehose-networks"
	"github.com/streamingfast/logging/zapx"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	xdscreds "google.golang.org/grpc/credentials/xds"
	_ "google.golang.org/grpc/encoding/gzip"
	stats "google.golang.org/grpc/stats"
	_ "google.golang.org/grpc/xds"
)

type AuthType int

const (
	None AuthType = iota
	JWT
	ApiKey
)

type ProtocolVersion int

const (
	ProtocolVersionUnset ProtocolVersion = 0
	ProtocolVersionV2    ProtocolVersion = 2
	ProtocolVersionV3    ProtocolVersion = 3
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
	default:
		return "unknown"
	}
}

// IsValid returns true if the protocol version is valid
func (pv ProtocolVersion) IsValid() bool {
	return pv == ProtocolVersionUnset || pv == ProtocolVersionV2 || pv == ProtocolVersionV3
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
	default:
		return ProtocolVersionUnset, fmt.Errorf("invalid protocol version %d, only 0, 2 and 3 are supported", version)
	}
}

func (pv ProtocolVersion) IsV2() bool {
	return pv == ProtocolVersionV2
}

func (pv ProtocolVersion) IsV3() bool {
	return pv == ProtocolVersionV3
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
	// AuthType specifies the type of authentication (None, JWT, or ApiKey)
	AuthType AuthType
	// Insecure allows insecure TLS connections (skips certificate verification)
	Insecure bool
	// PlainText uses unencrypted connections (no TLS)
	PlainText bool
	// Agent is the User-Agent string for gRPC requests
	Agent string
	// ForceProtocolVersion forces the use of a specific protocol version (2 or 3)
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

	return nil
}

type sizeLoggingHandler struct {
	messageCount int
	timeStart    time.Time

	uncompressedBytes int
	compressedBytes   int
	wireBytes         int
	lastReceivedTime  time.Time
	waitToReceive     time.Duration
}

func (h *sizeLoggingHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	zlog.Info("gRPC client RPC started", zap.String("method", info.FullMethodName))
	return ctx
}

func (h *sizeLoggingHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	if inPayload, ok := rs.(*stats.InPayload); ok && inPayload != nil {
		if h.messageCount == 0 {
			h.timeStart = time.Now()
		}
		h.waitToReceive += time.Since(h.lastReceivedTime)
		h.lastReceivedTime = time.Now()
		h.messageCount++
		h.uncompressedBytes += inPayload.Length
		h.compressedBytes += inPayload.CompressedLength
		h.wireBytes += inPayload.WireLength

		if h.messageCount == 10000 {
			secs := time.Since(h.timeStart).Seconds()
			messagesPerSecond := float64(h.messageCount) / secs
			compressedPercentage := 100.0 - (float64(h.compressedBytes) / float64(h.uncompressedBytes) * 100.0)

			zlog.Info(
				"grpc io stats",
				zap.Float64("msg_sec", messagesPerSecond),
				zapx.HumanDuration("duration", time.Since(h.timeStart)),
				zapx.HumanDuration("wait_to_receive", h.waitToReceive),
				zap.String("compression_ratio", fmt.Sprintf("%.2f%%", compressedPercentage)),
				zap.String("uncompressed", humanize.Bytes(uint64(h.uncompressedBytes))),
				zap.String("compressed", humanize.Bytes(uint64(h.compressedBytes))),
				zap.Int("uncompressed_bytes", h.uncompressedBytes),
				zap.Int("compressed_bytes", h.compressedBytes),
			)

			h.timeStart = time.Now()
			h.messageCount = 0
			h.uncompressedBytes = 0
			h.compressedBytes = 0
			h.wireBytes = 0
			h.waitToReceive = 0
		}
	}
}

func (h *sizeLoggingHandler) TagConn(ctx context.Context, cti *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *sizeLoggingHandler) HandleConn(ctx context.Context, cs stats.ConnStats) {
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
	skipAuth := authType == None || usePlainTextConnection
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

	sizeHandler := &sizeLoggingHandler{}
	dialOptions = append(dialOptions, grpc.WithStatsHandler(sizeHandler))

	zlog.Debug("getting connection", zap.String("endpoint", endpoint))
	conn, err := dgrpc.NewExternalClientConn(endpoint, dialOptions...)
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

	sizeHandler := &sizeLoggingHandler{}
	dialOptions = append(dialOptions, grpc.WithStatsHandler(sizeHandler))

	//compressor := os.Getenv("GRPC_COMPRESSOR")
	//switch compressor {
	//case "gzip":
	//dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)))
	//case "s2":
	dialOptions = append(dialOptions, grpc.WithDefaultCallOptions(grpc.UseCompressor(s2.Name)))
	//}

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
