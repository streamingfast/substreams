package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	compress "github.com/klauspost/connect-compress/v2"
	"github.com/streamingfast/dauth"
	dauthconnect "github.com/streamingfast/dauth/middleware/connect"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dgrpc/server/connectrpc"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcv2connect"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3/pbsubstreamsrpcv3connect"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4/pbsubstreamsrpcv4connect"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func connectServer(
	service *ConnectService,
	infoService pbsubstreamsrpcv2connect.EndpointInfoHandler,
	auth dauth.Authenticator,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
	logger *zap.Logger,
) *connectrpc.ConnectWebServer {

	tracerProvider := otel.GetTracerProvider()
	options := []dgrpcserver.Option{
		dgrpcserver.WithLogger(logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, healthcheck),
		dgrpcserver.WithGRPCServerOptions(
			grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracerProvider))),
			grpc.MaxRecvMsgSize(1024*1024*1024),
		),
	}
	options = append(options, dgrpcserver.WithConnectInterceptor(dauthconnect.NewAuthInterceptor(auth, logger)))
	options = append(options, dgrpcserver.WithConnectStrictContentType(false))
	options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv2connect.StreamName))
	options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv3connect.StreamName))
	options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv4connect.StreamName))

	streamHandlerGetter := func(opts ...connect.HandlerOption) (string, http.Handler) {
		var o []connect.HandlerOption
		for _, opt := range opts {
			o = append(o, opt)
		}
		o = append(o, compress.WithAll(compress.LevelBalanced))
		return pbsubstreamsrpcv2connect.NewStreamHandler(service, o...)
	}

	streamHandlerGetterV3 := func(opts ...connect.HandlerOption) (string, http.Handler) {
		handler := streamHandlerV3(service.BlocksV3)
		var o []connect.HandlerOption
		for _, opt := range opts {
			o = append(o, opt)
		}
		o = append(o, compress.WithAll(compress.LevelBalanced))
		return pbsubstreamsrpcv3connect.NewStreamHandler(handler, o...)
	}

	streamHandlerGetterV4 := func(opts ...connect.HandlerOption) (string, http.Handler) {
		handler := streamHandlerV4(service.BlocksV4)
		var o []connect.HandlerOption
		for _, opt := range opts {
			o = append(o, opt)
		}
		o = append(o, compress.WithAll(compress.LevelBalanced))
		return pbsubstreamsrpcv4connect.NewStreamHandler(handler, o...)
	}

	handlerGetters := []connectrpc.HandlerGetter{streamHandlerGetter, streamHandlerGetterV3, streamHandlerGetterV4}

	if infoService != nil {
		infoHandlerGetter := func(opts ...connect.HandlerOption) (string, http.Handler) {

			var o []connect.HandlerOption
			for _, opt := range opts {
				o = append(o, opt)
			}
			o = append(o, compress.WithAll(compress.LevelBalanced))
			out, outh := pbsubstreamsrpcv2connect.NewEndpointInfoHandler(infoService, o...)
			return out, outh
		}
		handlerGetters = append(handlerGetters, infoHandlerGetter)
	}

	options = append(options, dgrpcserver.WithConnectPermissiveCORS())
	server := connectrpc.New(handlerGetters, options...)
	server.OnTerminating(func(err error) {
		logger.Info("Tier1Service is terminating")
		server.Shutdown(time.Duration(0))
	})
	return server
}

type streamHandlerV3 func(ctx context.Context, req *connect.Request[pbsubstreamsrpcv3.Request], stream *connect.ServerStream[pbsubstreamsrpc.Response]) error

func (h streamHandlerV3) Blocks(ctx context.Context, req *connect.Request[pbsubstreamsrpcv3.Request], stream *connect.ServerStream[pbsubstreamsrpc.Response]) error {
	return h(ctx, req, stream)
}

type v3Adapter struct {
	*Tier1Service
}

func (a *v3Adapter) Blocks(req *pbsubstreamsrpcv3.Request, srv pbsubstreamsrpcv3.Stream_BlocksServer) error {
	return a.BlocksV3(req, srv)
}

type streamHandlerV4 func(ctx context.Context, req *connect.Request[pbsubstreamsrpcv3.Request], stream *connect.ServerStream[pbsubstreamsrpcv4.Response]) error

func (h streamHandlerV4) Blocks(ctx context.Context, req *connect.Request[pbsubstreamsrpcv3.Request], stream *connect.ServerStream[pbsubstreamsrpcv4.Response]) error {
	return h(ctx, req, stream)
}

type v4Adapter struct {
	*Tier1Service
}

func (a *v4Adapter) Blocks(req *pbsubstreamsrpcv3.Request, srv pbsubstreamsrpcv4.Stream_BlocksServer) error {
	return a.BlocksV4(req, srv)
}

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
	return s.inner.BlocksAnyConnect(ctx, req.Msg, req.Header(), pbsubstreamsrpcv2connect.StreamBlocksProcedure, nil, &serverStreamWrapper{stream, ctx}, false)
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

	return s.inner.BlocksAnyConnect(ctx, v2req, req.Header(), pbsubstreamsrpcv3connect.StreamBlocksProcedure, req.Msg.Package, &serverStreamWrapper{stream, ctx}, false)
}

func (s *ConnectService) BlocksV4(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpcv3.Request],
	stream *connect.ServerStream[pbsubstreamsrpcv4.Response],
) error {
	if req.Msg.Package == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("package is required"))
	}

	ctx = reqctx.WithSpkg(ctx, req.Msg.Package)

	v2req, err := req.Msg.ToV2()
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to convert request to v2: %w", err))
	}

	return s.inner.BlocksAnyConnect(ctx, v2req, req.Header(), pbsubstreamsrpcv3connect.StreamBlocksProcedure, req.Msg.Package, &serverStreamWrapperV4{stream, ctx}, true)
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

type serverStreamWrapperV4 struct {
	*connect.ServerStream[pbsubstreamsrpcv4.Response]
	ctx context.Context
}

func (w *serverStreamWrapperV4) SetHeader(metadata.MD) error {
	return nil
}

func (w *serverStreamWrapperV4) SendHeader(metadata.MD) error {
	return nil
}

func (w *serverStreamWrapperV4) SetTrailer(metadata.MD) {
}

func (w *serverStreamWrapperV4) Context() context.Context {
	return w.ctx
}

func (w *serverStreamWrapperV4) SendMsg(m interface{}) error {
	return w.ServerStream.Send(m.(*pbsubstreamsrpcv4.Response))
}

func (w *serverStreamWrapperV4) RecvMsg(m interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *Tier1Service) BlocksAnyConnect(
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	header http.Header,
	protocol string,
	pkg *pbsubstreams.Package,
	stream grpc.ServerStream,
	supportBuffering bool,
) (serverErr error) {

	logger := reqctx.Logger(ctx).Named("tier1-connect")
	runningCtx, err := s.BlocksAny(ctx, request, header, protocol, pkg, stream, supportBuffering, logger)

	if connectError := toConnectError(runningCtx, err); connectError != nil {
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

	return err
}
