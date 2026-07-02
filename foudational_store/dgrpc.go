package foudational_store

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/streamingfast/dgrpc"
	pbservice "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/service/v2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func NewStoreClient(rawEndpoint string, logger *zap.Logger) (
	client pbservice.StoreClient,
	closer func() error,
	err error,
) {
	logger = logger.Named("foundational-store")
	logger.Info("creating new foundational store", zap.String("raw_endpoint", rawEndpoint))

	transportCreds := insecure.NewCredentials()
	if u, err := url.Parse(rawEndpoint); err == nil {
		if u.Scheme == "grpcs" {
			transportCreds = credentials.NewTLS(&tls.Config{})
			rawEndpoint = u.Host
		} else if u.Scheme == "grpc" {
			rawEndpoint = u.Host
		} else if strings.HasSuffix(rawEndpoint, ":443") {
			// heuristic for bare host:443 -> use TLS (for hosted stores without scheme)
			transportCreds = credentials.NewTLS(&tls.Config{})
		}
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithBlock(),
		grpc.WithTimeout(5 * time.Second),
	}

	conn, err := dgrpc.NewInternalClientConn(rawEndpoint, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("creating new foundational store: %w", err)
	}

	c := func() error {
		return conn.Close()
	}
	return pbservice.NewStoreClient(conn), c, nil
}
