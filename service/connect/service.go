package connect

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcv2connect"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3/pbsubstreamsrpcv3connect"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Tier1Service interface {
	BlocksAny(
		ctx context.Context,
		request *pbsubstreamsrpc.Request,
		header http.Header,
		protocol string,
		pkg *pbsubstreams.Package,
		stream grpc.ServerStream,
	) error
}

type Service struct {
	inner Tier1Service
}

func NewService(inner Tier1Service) *Service {
	return &Service{inner: inner}
}

func (s *Service) Blocks(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpc.Request],
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) error {
	return s.inner.BlocksAny(ctx, req.Msg, req.Header(), pbsubstreamsrpcv2connect.StreamBlocksProcedure, nil, &serverStreamWrapper{stream, ctx})
}

func (s *Service) BlocksV3(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpcv3.Request],
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) error {
	if req.Msg.Package == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("package is required"))
	}

	ctx = reqctx.WithSpkg(ctx, req.Msg.Package)

	v2req, err := req.Msg.ToV2()
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to convert request to v2: %w", err))
	}

	return s.inner.BlocksAny(ctx, v2req, req.Header(), pbsubstreamsrpcv3connect.StreamBlocksProcedure, req.Msg.Package, &serverStreamWrapper{stream, ctx})
}

type serverStreamWrapper struct {
	*connect.ServerStream[pbsubstreamsrpc.Response]
	ctx context.Context
}

func (w *serverStreamWrapper) SetHeader(metadata.MD) error {
	return nil
}

func (w *serverStreamWrapper) SendHeader(metadata.MD) error {
	return nil
}

func (w *serverStreamWrapper) SetTrailer(metadata.MD) {
}

func (w *serverStreamWrapper) Context() context.Context {
	return w.ctx
}

func (w *serverStreamWrapper) SendMsg(m interface{}) error {
	return w.ServerStream.Send(m.(*pbsubstreamsrpc.Response))
}

func (w *serverStreamWrapper) RecvMsg(m interface{}) error {
	return fmt.Errorf("not implemented")
}
