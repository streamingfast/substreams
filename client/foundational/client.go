package foundational

import (
	"context"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/logging"
	pbstore "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Store struct {
	rpc    pbstore.StoreClient
	logger *zap.Logger
}

type Stores []*Store

func New(rawEndpoint string, logger *zap.Logger) (*Store, func() error, error) {
	logger = logger.Named("foundational-store")
	logger.Info("creating new foundational store", zap.String("raw_endpoint", rawEndpoint))

	if u, err := url.Parse(rawEndpoint); err == nil && (u.Scheme == "grpc" || u.Scheme == "grpcs") {
		rawEndpoint = u.Host
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		grpc.WithBlock(),
		grpc.WithTimeout(5 * time.Second),
	}

	conn, err := dgrpc.NewInternalClientConn(rawEndpoint, opts...)
	if err != nil {
		return nil, nil, err
	}

	return &Store{rpc: pbstore.NewStoreClient(conn), logger: logger}, conn.Close, nil
}

func (s *Store) Get(ctx context.Context, clock *pbsubstreams.Clock, key []byte) (*pbstore.GetResponse, error) {
	logging.Logger(ctx, s.logger).Debug("getting value from key")

	resp, err := s.rpc.Get(ctx, &pbstore.GetRequest{
		Key:         key,
		BlockNumber: clock.Number,
		BlockHash:   decodeHashString(clock.Id),
		OmitDeleted: true,
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *Store) GetAll(ctx context.Context, clock *pbsubstreams.Clock, keys [][]byte) (*pbstore.GetAllResponse, error) {
	logging.Logger(ctx, s.logger).Debug("getting values from keys")

	resp, err := s.rpc.GetAll(ctx, &pbstore.GetAllRequest{
		Keys:        keys,
		BlockNumber: clock.Number,
		BlockHash:   decodeHashString(clock.Id),
		OmitDeleted: true,
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Attempts to decode a hash string into bytes using multiple formats
func decodeHashString(hashStr string) []byte {
	if hashStr == "" {
		return nil
	}

	// hex with 0x prefix
	if strings.HasPrefix(hashStr, "0x") && len(hashStr) == 66 {
		if decoded, err := hex.DecodeString(hashStr[2:]); err == nil {
			return decoded
		}
	}

	//hex without prefix
	if len(hashStr) == 64 {
		if decoded, err := hex.DecodeString(strings.ToLower(hashStr)); err == nil {
			return decoded
		}
	}

	// base58
	if decoded, err := base58.Decode(hashStr); err == nil && len(decoded) == 32 {
		return decoded
	}

	// Fallback: return nil if no format worked
	return nil
}
