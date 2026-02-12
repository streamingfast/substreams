package service

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
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type ConnectService struct {
	inner *Tier1Service
}

func NewService(inner *Tier1Service) *ConnectService {
	return &ConnectService{inner: inner}
}

func (s *ConnectService) Blocks(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpc.Request],
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) error {
	return s.inner.BlocksAnyConnect(ctx, req.Msg, req.Header(), pbsubstreamsrpcv2connect.StreamBlocksProcedure, nil, &serverStreamWrapper{stream, ctx})
}

func (s *ConnectService) BlocksV3(
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

	return s.inner.BlocksAnyConnect(ctx, v2req, req.Header(), pbsubstreamsrpcv3connect.StreamBlocksProcedure, req.Msg.Package, &serverStreamWrapper{stream, ctx})
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

func (s *Tier1Service) BlocksAnyConnect(
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	header http.Header,
	protocol string,
	pkg *pbsubstreams.Package,
	stream grpc.ServerStream,
) (serverErr error) {

	logger := reqctx.Logger(ctx).Named("tier1-connect")
	runningCtx, err, blockErr := s.BlocksAny(ctx, request, header, protocol, pkg, stream, logger)
	if err != nil {
		return err
	}

	if connectError := toConnectError(runningCtx, blockErr); connectError != nil {
		switch connect.CodeOf(connectError) {
		case connect.CodeInternal:
			logger.Warn("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
		case connect.CodeInvalidArgument:
			logger.Debug("invalid argument on request", zap.Error(connectError))
		case connect.CodeCanceled:
			logger.Debug("Blocks request canceled by user", zap.Error(connectError))
		case connect.CodeResourceExhausted:
			logger.Debug("Blocks request failed with ResourceExhausted", zap.Error(connectError))
		default:
			logger.Warn("Blocks request completed with error", zap.Error(connectError))
		}
		return connectError
	}

	return blockErr
}
