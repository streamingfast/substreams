package service

import (
	"context"
	"net/http"

	"github.com/streamingfast/dauth"
	dauthgrpc "github.com/streamingfast/dauth/middleware/grpc"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dgrpc/server/standard"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func grpcServer(
	service *Tier1Service,
	infoService pbsubstreamsrpc.EndpointInfoServer,
	auth dauth.Authenticator,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
	logger *zap.Logger,
) *standard.StandardServer {
	options := &dgrpcserver.Options{
		Logger:          logger,
		HealthCheck:     healthcheck,
		HealthCheckOver: dgrpcserver.HealthCheckOverGRPC | dgrpcserver.HealthCheckOverHTTP,
		ServerOptions: []grpc.ServerOption{
			grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(otel.GetTracerProvider()))),
			grpc.MaxRecvMsgSize(1024 * 1024 * 1024),
		},
		EnforceCompression:     enforceCompression,
		PostUnaryInterceptors:  []grpc.UnaryServerInterceptor{dauthgrpc.UnaryAuthChecker(auth, logger)},
		PostStreamInterceptors: []grpc.StreamServerInterceptor{dauthgrpc.StreamAuthChecker(auth, logger)},
	}

	server := standard.NewServer(options)

	pbsubstreamsrpc.RegisterStreamServer(server.ServiceRegistrar(), service)
	pbsubstreamsrpcv3.RegisterStreamServer(server.ServiceRegistrar(), &v3Adapter{service})
	pbsubstreamsrpcv4.RegisterStreamServer(server.ServiceRegistrar(), &v4Adapter{service})
	pbsubstreamsrpcv4.RegisterEstimatorServer(server.ServiceRegistrar(), service)
	if infoService != nil {
		pbsubstreamsrpc.RegisterEndpointInfoServer(server.ServiceRegistrar(), infoService)
	}

	return server
}

func (s *Tier1Service) Blocks(req *pbsubstreamsrpc.Request, srv pbsubstreamsrpc.Stream_BlocksServer) error {
	ctx := srv.Context()
	header := metadataToHeader(ctx)
	protocol := "/sf.substreams.rpc.v2.Stream/Blocks"
	return s.BlocksAnyGrpc(ctx, req, header, protocol, nil, srv, false)
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
	return s.BlocksAnyGrpc(ctx, v2req, header, protocol, req.Package, srv, false)
}

func (s *Tier1Service) BlocksV4(req *pbsubstreamsrpcv3.Request, srv pbsubstreamsrpcv4.Stream_BlocksServer) error {
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
	return s.BlocksAnyGrpc(ctx, v2req, header, protocol, req.Package, srv, true)
}

// Estimate implements the sf.substreams.rpc.v4.Estimator service over gRPC.
func (s *Tier1Service) Estimate(req *pbsubstreamsrpcv4.EstimateRequest, srv grpc.ServerStreamingServer[pbsubstreamsrpcv4.EstimateResponse]) error {
	ctx := srv.Context()
	err := s.estimate(ctx, req, metadataToHeader(ctx), srv.Send)
	if grpcErr := toGrpcTier1Error(ctx, err); grpcErr != nil {
		s.logger.Debug("Estimate request completed with error", zap.Error(grpcErr))
		return grpcErr
	}
	return nil
}

func (s *Tier1Service) BlocksAnyGrpc(
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	header http.Header,
	protocol string,
	pkg *pbsubstreams.Package,
	stream grpc.ServerStream,
	supportBuffering bool,
) (serverErr error) {

	logger := reqctx.Logger(ctx).Named("tier1-grpc")
	runningCtx, err := s.BlocksAny(ctx, request, header, protocol, pkg, stream, supportBuffering, logger)

	if grpcErr := toGrpcTier1Error(runningCtx, err); grpcErr != nil {
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

	return err
}
