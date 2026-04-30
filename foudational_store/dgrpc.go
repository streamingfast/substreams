package foudational_store

import (
	"fmt"
	"net/url"
	"time"

	"github.com/streamingfast/dgrpc"
	pbservice "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/service/v2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewStoreClient(rawEndpoint string, logger *zap.Logger) (
	client pbservice.StoreClient,
	closer func() error,
	err error,
) {
	logger = logger.Named("foundational-store")
	logger.Info("creating new foundational store", zap.String("raw_endpoint", rawEndpoint))

	if u, err := url.Parse(rawEndpoint); err == nil && (u.Scheme == "grpc" || u.Scheme == "grpcs") {
		rawEndpoint = u.Host
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
