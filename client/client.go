package client

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/streamingfast/dgrpc"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	xdscreds "google.golang.org/grpc/credentials/xds"
	_ "google.golang.org/grpc/xds"
)

type SubstreamsClientConfig struct {
	endpoint  string
	jwt       string
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

func (c *SubstreamsClientConfig) JWT() string {
	return c.jwt
}

type Factory = func() (cli pbsubstreams.StreamClient, closeFunc func() error, callOpts []grpc.CallOption, err error)

func NewSubstreamsClientConfig(endpoint string, jwt string, insecure bool, plaintext bool) *SubstreamsClientConfig {
	return &SubstreamsClientConfig{
		endpoint:  endpoint,
		jwt:       jwt,
		insecure:  insecure,
		plaintext: plaintext,
	}
}

var portSuffixRegex = regexp.MustCompile(":[0-9]{2,5}$")

func NewFactory(config *SubstreamsClientConfig) Factory {
	bootStrapFilename := os.Getenv("GRPC_XDS_BOOTSTRAP")

	if bootStrapFilename == "" {
		zlog.Info("setting up basic grpc client factory (no XDS bootstrap)")

		return func() (cli pbsubstreams.StreamClient, closeFunc func() error, callOpts []grpc.CallOption, err error) {
			return NewSubstreamsClient(config)
		}
	}

	zlog.Info("setting up xds grpc client factory", zap.String("GRPC_XDS_BOOTSTRAP", bootStrapFilename))

	noop := func() error { return nil }
	cli, _, callOpts, err := NewSubstreamsClient(config)
	return func() (pbsubstreams.StreamClient, func() error, []grpc.CallOption, error) {
		return cli, noop, callOpts, err
	}
}

func NewSubstreamsClient(config *SubstreamsClientConfig) (cli pbsubstreams.StreamClient, closeFunc func() error, callOpts []grpc.CallOption, err error) {
	if config == nil {
		return nil, nil, nil, fmt.Errorf("substreams client config not set")
	}
	endpoint := config.endpoint
	jwt := config.jwt
	usePlainTextConnection := config.plaintext
	useInsecureTLSConnection := config.insecure

	if !portSuffixRegex.MatchString(endpoint) {
		return nil, nil, nil, fmt.Errorf("invalid endpoint %q: endpoint's suffix must be a valid port in the form ':<port>', port 443 is usually the right one to use", endpoint)
	}

	bootStrapFilename := os.Getenv("GRPC_XDS_BOOTSTRAP")

	var dialOptions []grpc.DialOption
	skipAuth := jwt == "" || usePlainTextConnection
	if bootStrapFilename != "" {
		log.Println("Using xDS credentials...")
		creds, err := xdscreds.NewClientCredentials(xdscreds.ClientOptions{FallbackCreds: insecure.NewCredentials()})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create xDS credentials: %v", err)
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else {
		if useInsecureTLSConnection && usePlainTextConnection {
			return nil, nil, nil, fmt.Errorf("option --insecure and --plaintext are mutually exclusive, they cannot be both specified at the same time")
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
		return nil, nil, nil, fmt.Errorf("unable to create external gRPC client: %w", err)
	}
	closeFunc = conn.Close

	if !skipAuth {
		zlog.Debug("creating oauth access", zap.String("endpoint", endpoint))
		creds := oauth.NewOauthAccess(&oauth2.Token{AccessToken: jwt, TokenType: "Bearer"})
		callOpts = append(callOpts, grpc.PerRPCCredentials(creds))
	}

	zlog.Debug("creating new client", zap.String("endpoint", endpoint))
	cli = pbsubstreams.NewStreamClient(conn)
	zlog.Debug("client created")
	return
}
