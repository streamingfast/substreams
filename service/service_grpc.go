package service

import (
	"context"
	"net/http"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Tier1Service) Blocks(req *pbsubstreamsrpc.Request, srv pbsubstreamsrpc.Stream_BlocksServer) error {
	ctx := srv.Context()
	header := metadataToHeader(ctx)
	protocol := "/sf.substreams.rpc.v2.Stream/Blocks"
	return s.BlocksAnyGrpc(ctx, req, header, protocol, nil, srv)
}

func (s *Tier1Service) BlocksV3(req *pbsubstreamsrpcv3.Request, srv pbsubstreamsrpcv3.Stream_BlocksServer) error {
	_, err := manifest.ApplyPackageTransformations(req.Package, false, req.Network, req.OutputModule, req.Params)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	ctx := srv.Context()
	ctx = reqctx.WithSpkg(ctx, req.Package)
	v2req, err := req.ToV2()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to convert request to v2: %s", err)
	}
	header := metadataToHeader(ctx)
	protocol := "/sf.substreams.rpc.v3.Stream/Blocks"
	return s.BlocksAnyGrpc(ctx, v2req, header, protocol, req.Package, srv)
}

func (s *Tier1Service) BlocksAnyGrpc(
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	header http.Header,
	protocol string,
	pkg *pbsubstreams.Package,
	stream grpc.ServerStream,
) (serverErr error) {

	logger := reqctx.Logger(ctx).Named("tier1-grpc")
	runningCtx, err, blockErr := s.BlocksAny(ctx, request, header, protocol, pkg, stream, logger)
	if err != nil {
		return err
	}

	if grpcErr := toGrpcTier1Error(runningCtx, blockErr); grpcErr != nil {
		switch status.Code(grpcErr) {
		case codes.Internal:
			logger.Warn("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
		case codes.InvalidArgument:
			logger.Debug("invalid argument on request", zap.Error(grpcErr))
		case codes.Canceled:
			logger.Debug("Blocks request canceled by user", zap.Error(grpcErr))
		case codes.ResourceExhausted:
			logger.Debug("Blocks request failed with ResourceExhausted", zap.Error(grpcErr))
		default:
			logger.Warn("Blocks request completed with error", zap.Error(grpcErr))
		}
		return grpcErr
	}

	return blockErr
}
