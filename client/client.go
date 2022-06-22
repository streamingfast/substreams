package client

import (
	"crypto/tls"
	"fmt"

	"go.uber.org/zap"

	"github.com/streamingfast/dgrpc"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
)

func NewSubstreamsClient(endpoint, jwt string, useInsecureTLSConnection, usePlainTextConnection bool) (cli pbsubstreams.StreamClient, closeFunc func() error, callOpts []grpc.CallOption, err error) {
	zlog.Debug("creating new client", zap.String("endpoint", endpoint), zap.Bool("jwt_present", jwt != ""))
	skipAuth := jwt == "" || usePlainTextConnection

	if useInsecureTLSConnection && usePlainTextConnection {
		return nil, nil, nil, fmt.Errorf("option --insecure and --plaintext are mutually exclusive, they cannot be both specified at the same time")
	}

	var dialOptions []grpc.DialOption
	switch {
	case usePlainTextConnection:
		zlog.Debug("setting plain text option")
		skipAuth = true
		dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	case useInsecureTLSConnection:
		zlog.Debug("setting insecure tls connection option")
		dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))}
	}

	zlog.Debug("getting connection", zap.String("endpoint", endpoint))
	conn, err := dgrpc.NewExternalClient(endpoint, dialOptions...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to create external gRPC client")
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
