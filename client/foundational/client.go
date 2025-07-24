package foundational

import (
	"context"
	"net/url"
	"time"

	"github.com/streamingfast/dgrpc"
	pbstore "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Store struct {
	rpc pbstore.StoreKVClient
}

type Stores []*Store

func New(rawEndpoint string) (*Store, func() error, error) {
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

	conn, err := dgrpc.NewExternalClientConn(rawEndpoint, opts...)
	if err != nil {
		return nil, nil, err
	}

	return &Store{rpc: pbstore.NewStoreKVClient(conn)}, conn.Close, nil
}

func (s *Store) Get(ctx context.Context, block uint64, key []byte) (*pbstore.GetResponse, error) {
	resp, err := s.rpc.Get(ctx, &pbstore.GetRequest{
		Key:         key,
		BlockNumber: block,
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *Store) GetAll(ctx context.Context, block uint64, keys [][]byte) (*pbstore.GetAllResponse, error) {
	resp, err := s.rpc.GetAll(ctx, &pbstore.GetAllRequest{
		Keys:        keys,
		BlockNumber: block,
		OmitDeleted: true,
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}
