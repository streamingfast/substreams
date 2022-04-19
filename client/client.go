package client

import (
	"crypto/tls"
	"fmt"

	"github.com/streamingfast/dgrpc"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
)

func NewSubstreamsClient(endpoint, jwt string, useInsecureTSLConnection, usePlainTextConnection bool) (pbsubstreams.StreamClient, []grpc.CallOption, error) {
	skipAuth := jwt == ""

	if useInsecureTSLConnection && usePlainTextConnection {
		return nil, nil, fmt.Errorf("option --insecure and --plaintext are mutually exclusive, they cannot be both specified at the same time")
	}

	var dialOptions []grpc.DialOption
	switch {
	case usePlainTextConnection:
		dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	case useInsecureTSLConnection:
		dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))}
	}

	conn, err := dgrpc.NewExternalClient(endpoint, dialOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create external gRPC client")
	}

	var callOpts []grpc.CallOption
	if !skipAuth {
		credentials := oauth.NewOauthAccess(&oauth2.Token{AccessToken: jwt, TokenType: "Bearer"})
		callOpts = append(callOpts, grpc.PerRPCCredentials(credentials))
	}

	return pbsubstreams.NewStreamClient(conn), callOpts, nil
}
