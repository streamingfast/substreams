package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	compress "github.com/klauspost/connect-compress/v2"
	_ "github.com/mostynb/go-grpc-compression/experimental/s2"
	_ "github.com/mostynb/go-grpc-compression/lz4"
	_ "github.com/mostynb/go-grpc-compression/zstd"
	vt "github.com/planetscale/vtprotobuf/codec/grpc"
	"github.com/streamingfast/dauth"
	dauthconnect "github.com/streamingfast/dauth/middleware/connect"
	dauthgrpc "github.com/streamingfast/dauth/middleware/grpc"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	connectweb "github.com/streamingfast/dgrpc/server/connectrpc"
	"github.com/streamingfast/dgrpc/server/factory"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcv2connect"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3/pbsubstreamsrpcv3connect"
	tier1Connect "github.com/streamingfast/substreams/service/connect"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/experimental"
)

func init() {
	fmt.Println("------------------ VT Proto registered ------------------------")
	encoding.RegisterCodec(vt.Codec{})
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

func ListenTier1(
	secureProxyListenAddress string,
	plaintextProxyListenAddress string,
	secureGrpcListenAddress string,
	plaintextGrpcListenAddress string,
	secureConnectListenAddress string,
	plaintextConnectListenAddress string,
	svc *Tier1Service,
	connectSvc *tier1Connect.Service,
	infoService pbsubstreamsrpc.EndpointInfoServer,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
) (err error) {

	var secureGrpcServer dgrpcserver.Server
	var plaintextGrpcServer dgrpcserver.Server
	var secureConnectServer dgrpcserver.Server
	var plaintextConnectServer dgrpcserver.Server
	if plaintextGrpcListenAddress != "" {
		plaintextGrpcServer = grpcSever(plaintextGrpcListenAddress, svc, infoService, auth, healthcheck, enforceCompression, logger)
		go func() {
			logger.Info("starting grpc server", zap.String("address", plaintextGrpcListenAddress))
			plaintextGrpcServer.Launch(strings.ReplaceAll(plaintextGrpcListenAddress, "*", ""))
		}()
	}

	if secureGrpcListenAddress != "" {
		secureGrpcServer = grpcSever(secureGrpcListenAddress, svc, infoService, auth, healthcheck, enforceCompression, logger)
		go func() {
			logger.Info("starting grpc server", zap.String("address", secureGrpcListenAddress))
			secureGrpcServer.Launch(strings.ReplaceAll(secureGrpcListenAddress, "*", ""))
		}()
	}

	if plaintextConnectListenAddress != "" {
		plaintextConnectServer = connectServer(plaintextConnectListenAddress, connectSvc, infoService, auth, healthcheck, enforceCompression, logger)
		go func() {
			logger.Info("starting connect server", zap.String("address", plaintextConnectListenAddress))
			plaintextConnectServer.Launch(strings.ReplaceAll(plaintextConnectListenAddress, "*", ""))
		}()
	}

	if secureConnectListenAddress != "" {
		secureConnectServer = connectServer(secureConnectListenAddress, connectSvc, infoService, auth, healthcheck, enforceCompression, logger)
		go func() {
			logger.Info("starting connect server", zap.String("address", secureConnectListenAddress))
			secureConnectServer.Launch(strings.ReplaceAll(secureConnectListenAddress, "*", ""))
		}()
	}

	// The main multiplexer / router
	mux := http.NewServeMux()

	// Catch-all handler that decides based on Content-Type
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		isSecure := r.TLS != nil

		// gRPC protocol starts with application/grpc
		// gRPC-Web often uses application/grpc-web or application/grpc-web+proto
		if strings.HasPrefix(contentType, "application/grpc") ||
			strings.HasPrefix(contentType, "application/grpc-web") {

			if isSecure {
				logger.Debug("forwarding gRPC request over HTTPS")
				secureGrpcServer.ServeHTTP(w, r)
				return
			}
			logger.Debug("forwarding gRPC request over HTTP")
			plaintextGrpcServer.ServeHTTP(w, r)
			return
		}

		// Connect protocol: application/connect+proto or application/connect+json
		// Also catch JSON for REST-like testing
		if strings.HasPrefix(contentType, "application/connect") ||
			contentType == "application/json" ||
			strings.Contains(contentType, "json") { // loose match for safety

			if isSecure {
				logger.Debug("forwarding gRPC-Web request over HTTPS")
				secureConnectServer.ServeHTTP(w, r)
				return
			}

			logger.Debug("forwarding gRPC-Web request over HTTP")
			plaintextConnectServer.ServeHTTP(w, r)

			return
		}

		// Fallback: could return 415 or route to one by default
		http.Error(w, "Unsupported Content-Type for RPC", http.StatusUnsupportedMediaType)
	})

	// Support HTTP/1.1 → h2c upgrade + direct h2
	handler := h2c.NewHandler(mux, &http2.Server{})

	logger.Info("starting secure proxy server", zap.String("address", secureProxyListenAddress))
	go func() {
		if err := http.ListenAndServe(strings.ReplaceAll(secureProxyListenAddress, "*", ""), handler); err != nil {
			logger.Error("failed to start secure proxy server", zap.Error(err))
		}
	}()
	logger.Info("starting plaintext proxy server", zap.String("address", plaintextProxyListenAddress))
	go func() {
		if err := http.ListenAndServe(strings.ReplaceAll(plaintextProxyListenAddress, "*", ""), handler); err != nil {
			logger.Error("failed to start secure proxy server", zap.Error(err))
		}
	}()

	logger.Info("started")

	if secureGrpcServer != nil {
		<-secureGrpcServer.Terminating()
		logger.Info("secure gRPC server terminated", zap.Error(secureGrpcServer.Error()))
	}
	if plaintextGrpcServer != nil {
		<-plaintextGrpcServer.Terminating()
		logger.Info("secure gRPC server terminated", zap.Error(plaintextGrpcServer.Error()))
	}
	if secureConnectServer != nil {
		<-secureConnectServer.Terminating()
		logger.Info("secure gRPC server terminated", zap.Error(secureConnectServer.Error()))
	}
	if plaintextConnectServer != nil {
		<-plaintextConnectServer.Terminating()
		logger.Info("secure gRPC server terminated", zap.Error(plaintextConnectServer.Error()))
	}

	return
}

func grpcSever(
	address string,
	service *Tier1Service,
	infoService pbsubstreamsrpc.EndpointInfoServer,
	auth dauth.Authenticator,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
	logger *zap.Logger,
) dgrpcserver.Server {
	options := GetCommonServerOptions(address, logger, healthcheck, enforceCompression)
	options = append(options, dgrpcserver.WithPostUnaryInterceptor(dauthgrpc.UnaryAuthChecker(auth, logger)))
	options = append(options, dgrpcserver.WithPostStreamInterceptor(dauthgrpc.StreamAuthChecker(auth, logger)))

	server := factory.ServerFromOptions(options...)
	pbsubstreamsrpc.RegisterStreamServer(server.ServiceRegistrar(), service)
	pbsubstreamsrpcv3.RegisterStreamServer(server.ServiceRegistrar(), &v3Adapter{service})
	if infoService != nil {
		pbsubstreamsrpc.RegisterEndpointInfoServer(server.ServiceRegistrar(), infoService)
	}

	cleanAddr := strings.ReplaceAll(address, "*", "")

	service.OnTerminating(func(err error) {
		logger.Info("Tier1Service is terminating", zap.String("address", cleanAddr), zap.Error(err))
		server.Shutdown(0)
	})
	return server

}

func connectServer(
	address string,
	service *tier1Connect.Service,
	infoService pbsubstreamsrpc.EndpointInfoServer,
	auth dauth.Authenticator,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
	logger *zap.Logger,
) dgrpcserver.Server {
	options := GetConnectCommonServerOptions(address, logger, healthcheck, enforceCompression)

	options = append(options, dgrpcserver.WithConnectInterceptor(dauthconnect.NewAuthInterceptor(auth, logger)))
	options = append(options, dgrpcserver.WithConnectStrictContentType(false))
	options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv2connect.StreamName))
	options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv3connect.StreamName))
	options = append(options, dgrpcserver.WithConnectReflection(pbsubstreamsrpcv3connect.StreamName))

	//todo: move compression to dgrpc :-(

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

	handlerGetters := []connectweb.HandlerGetter{streamHandlerGetter, streamHandlerGetterV3}

	//GRRRRRRRRRRRRRRRRRRRR
	//GRRRRRRRRRRRRRRRRRRRR
	// FIX ME!
	//GRRRRRRRRRRRRRRRRRRRR
	//GRRRRRRRRRRRRRRRRRRRR
	//if infoService != nil {
	//	infoHandlerGetter := func(opts ...connect.HandlerOption) (string, http.Handler) {
	//
	//		var o []connect.HandlerOption
	//		for _, opt := range opts {
	//			o = append(o, opt)
	//		}
	//		o = append(o, compress.WithAll(compress.LevelBalanced))
	//		out, outh := pbsubstreamsrpcv2connect.NewEndpointInfoHandler(infoService, o...)
	//		return out, outh
	//	}
	//	handlerGetters = append(handlerGetters, infoHandlerGetter)
	//}

	options = append(options, dgrpcserver.WithConnectPermissiveCORS())
	server := connectweb.New(handlerGetters, options...)
	server.OnTerminating(func(err error) {
		logger.Info("Tier1Service is terminating")
		server.Shutdown(time.Duration(0))
	})
	return server
}

func ListenTier2(
	addr string,
	serviceDiscoveryURL *url.URL,
	svc *Tier2Service,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
) (err error) {
	options := GetCommonServerOptions(addr, logger, healthcheck, enforceCompression)
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

func GetCommonServerOptions(listenAddr string, logger *zap.Logger, healthcheck dgrpcserver.HealthCheck, enforceCompression bool) []dgrpcserver.Option {
	tracerProvider := otel.GetTracerProvider()
	options := []dgrpcserver.Option{

		dgrpcserver.WithLogger(logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, healthcheck),
		dgrpcserver.WithGRPCServerOptions(
			grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracerProvider))),
			grpc.MaxRecvMsgSize(1024*1024*1024),
		),
	}
	if enforceCompression {
		options = append(options, dgrpcserver.WithEnforceCompression())
	}

	if strings.Contains(listenAddr, "*") {
		options = append(options, dgrpcserver.WithInsecureServer())
	} else {
		options = append(options, dgrpcserver.WithPlainTextServer())
	}
	return options
}

func GetConnectCommonServerOptions(listenAddr string, logger *zap.Logger, healthcheck dgrpcserver.HealthCheck, enforceCompression bool) []dgrpcserver.Option {
	tracerProvider := otel.GetTracerProvider()
	options := []dgrpcserver.Option{
		dgrpcserver.WithLogger(logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, healthcheck),
		dgrpcserver.WithGRPCServerOptions(
			grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracerProvider))),
			grpc.MaxRecvMsgSize(1024*1024*1024),
		),
	}
	if strings.Contains(listenAddr, "*") {
		options = append(options, dgrpcserver.WithInsecureServer())
	} else {
		options = append(options, dgrpcserver.WithPlainTextServer())
	}
	return options
}
