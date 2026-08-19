package foudational_store

import (
	"fmt"
	"net/url"
	"time"

	"github.com/streamingfast/dgrpc"
	pbservice "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/service/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func NewStoreClient(rawEndpoint string, useTLS bool, logger *zap.Logger) (
	client pbservice.StoreClient,
	closer func() error,
	err error,
) {
	logger = logger.Named("foundational-store")
	logger.Info("creating new foundational store", zap.String("raw_endpoint", rawEndpoint), zap.Bool("tls", useTLS))

	if u, err := url.Parse(rawEndpoint); err == nil && (u.Scheme == "grpcs" || u.Scheme == "grpc") {
		useTLS = u.Scheme == "grpcs"
		rawEndpoint = u.Host
	}

	creds, err := dgrpc.WithAutoTransportCredentials(false, !useTLS, false)
	if err != nil {
		return nil, nil, fmt.Errorf("foundational store transport credentials: %w", err)
	}

	conn, err := dgrpc.NewClientConn(rawEndpoint, creds, grpc.WithBlock(), grpc.WithTimeout(5*time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("creating new foundational store: %w", err)
	}

	c := func() error {
		return conn.Close()
	}
	return pbservice.NewStoreClient(conn), c, nil
}
