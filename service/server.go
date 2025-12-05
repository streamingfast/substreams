package service

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	connect_go "connectrpc.com/connect"
	"github.com/streamingfast/dauth"
	dauthconnect "github.com/streamingfast/dauth/middleware/connect"
	dauthgrpc "github.com/streamingfast/dauth/middleware/grpc"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	connectweb "github.com/streamingfast/dgrpc/server/connectrpc"
	"github.com/streamingfast/dgrpc/server/factory"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv2connect "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcv2connect"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv3connect "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3/pbsubstreamsrpcv3connect"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func GetCommonServerOptions(listenAddr string, logger *zap.Logger, healthcheck dgrpcserver.HealthCheck) []dgrpcserver.Option {
	tracerProvider := otel.GetTracerProvider()
	options := []dgrpcserver.Option{
		dgrpcserver.WithLogger(logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, healthcheck),
		dgrpcserver.WithPostUnaryInterceptor(otelgrpc.UnaryServerInterceptor(otelgrpc.WithTracerProvider(tracerProvider))),
		dgrpcserver.WithPostStreamInterceptor(otelgrpc.StreamServerInterceptor(otelgrpc.WithTracerProvider(tracerProvider))),
		dgrpcserver.WithGRPCServerOptions(grpc.MaxRecvMsgSize(1024 * 1024 * 1024)),
	}
	if strings.Contains(listenAddr, "*") {
		options = append(options, dgrpcserver.WithInsecureServer())
	} else {
		options = append(options, dgrpcserver.WithPlainTextServer())
	}
	return options
}

type streamHandlerV3 func(ctx context.Context, req *connect_go.Request[pbsubstreamsrpcv3.Request], stream *connect_go.ServerStream[pbsubstreamsrpc.Response]) error

func (h streamHandlerV3) Blocks(ctx context.Context, req *connect_go.Request[pbsubstreamsrpcv3.Request], stream *connect_go.ServerStream[pbsubstreamsrpc.Response]) error {
	return h(ctx, req, stream)
}

func ListenTier1(
	listenAddr string,
	svc *Tier1Service,
	infoService pbsubstreamsrpcv2connect.EndpointInfoHandler,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthcheck dgrpcserver.HealthCheck,
) (err error) {

	done := make(chan struct{})
	var servers []*connectweb.ConnectWebServer
	for _, addr := range strings.Split(listenAddr, ",") {
		// note: some of these common options don't work with connectWeb
		options := GetCommonServerOptions(addr, logger, healthcheck)

		options = append(options, dgrpcserver.WithConnectInterceptor(dauthconnect.NewAuthInterceptor(auth, logger)))
		options = append(options, dgrpcserver.WithConnectStrictContentType(false))
		options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv2connect.StreamName))
		options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv3connect.StreamName))

		streamHandlerGetter := func(opts ...connect_go.HandlerOption) (string, http.Handler) {
			return pbsubstreamsrpcv2connect.NewStreamHandler(svc, opts...)
		}

		streamHandlerGetterV3 := func(opts ...connect_go.HandlerOption) (string, http.Handler) {
			handler := streamHandlerV3(svc.BlocksV3)
			return pbsubstreamsrpcv3connect.NewStreamHandler(handler, opts...)
		}

		handlerGetters := []connectweb.HandlerGetter{streamHandlerGetter, streamHandlerGetterV3}

		if infoService != nil {
			infoHandlerGetter := func(opts ...connect_go.HandlerOption) (string, http.Handler) {
				out, outh := pbsubstreamsrpcv2connect.NewEndpointInfoHandler(infoService, opts...)
				return out, outh
			}
			handlerGetters = append(handlerGetters, infoHandlerGetter)
		}

		options = append(options, dgrpcserver.WithConnectPermissiveCORS())
		srv := connectweb.New(handlerGetters, options...)
		svc.OnTerminating(func(err error) {
			logger.Info("Tier1Service is terminating")
			srv.Shutdown(err)
		})
		servers = append(servers, srv)
		cleanAddr := strings.ReplaceAll(addr, "*", "")
		go func() {
			srv.Launch(cleanAddr)
			done <- struct{}{}
		}()
	}

	<-done
	for _, srv := range servers {
		srv.Shutdown(nil)
	}

	for _, srv := range servers {
		<-srv.Terminated()
		if e := srv.Err(); e != nil {
			err = e
		}
	}

	return
}

func ListenTier2(
	addr string,
	serviceDiscoveryURL *url.URL,
	svc *Tier2Service,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthcheck dgrpcserver.HealthCheck,
) (err error) {
	options := GetCommonServerOptions(addr, logger, healthcheck)
	if serviceDiscoveryURL != nil {
		options = append(options, dgrpcserver.WithServiceDiscoveryURL(serviceDiscoveryURL))
	}
	options = append(options,
		dgrpcserver.WithPostUnaryInterceptor(dauthgrpc.UnaryAuthChecker(auth, logger)),
		dgrpcserver.WithPostStreamInterceptor(dauthgrpc.StreamAuthChecker(auth, logger)),
	)

	grpcServer := factory.ServerFromOptions(options...)
	pbssinternal.RegisterSubstreamsServer(grpcServer.ServiceRegistrar(), svc)

	svc.OnTerminating(func(err error) {
		logger.Info("Tier2Service is terminating")
		grpcServer.Shutdown(0)
	})

	done := make(chan struct{})
	grpcServer.OnTerminated(func(e error) {
		err = e
		close(done)
	})
	addr = strings.ReplaceAll(addr, "*", "")
	grpcServer.Launch(addr)
	<-done

	return

}
