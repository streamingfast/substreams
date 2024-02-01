package client

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/streamingfast/dgrpc"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	xdscreds "google.golang.org/grpc/credentials/xds"
	_ "google.golang.org/grpc/xds"
)

type AuthType int

const (
	None AuthType = iota
	JWT
	ApiKey
)

type SubstreamsClientConfig struct {
	endpoint  string
	authToken string
	authType  AuthType
	insecure  bool
	plaintext bool
}

func (c *SubstreamsClientConfig) Endpoint() string {
	return c.endpoint
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

type InternalClientFactory = func() (cli pbssinternal.SubstreamsClient, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error)

func NewSubstreamsClientConfig(endpoint string, authToken string, authType AuthType, insecure bool, plaintext bool) *SubstreamsClientConfig {
	return &SubstreamsClientConfig{
		endpoint:  endpoint,
		authToken: authToken,
		authType:  authType,
		insecure:  insecure,
		plaintext: plaintext,
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

	dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	dialOptions = append(dialOptions, grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()))

	zlog.Debug("getting connection", zap.String("endpoint", endpoint))
	conn, err := dgrpc.NewExternalClient(endpoint, dialOptions...)
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

func NewSubstreamsClient(config *SubstreamsClientConfig) (cli pbsubstreamsrpc.StreamClient, closeFunc func() error, callOpts []grpc.CallOption, headers Headers, err error) {
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

	dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	dialOptions = append(dialOptions, grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()))

	zlog.Debug("getting connection", zap.String("endpoint", endpoint))
	conn, err := dgrpc.NewExternalClient(endpoint, dialOptions...)
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
	cli = pbsubstreamsrpc.NewStreamClient(conn)
	zlog.Debug("client created")
	return
}
