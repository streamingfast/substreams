package service

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	_ "github.com/mostynb/go-grpc-compression/experimental/s2"
	_ "github.com/mostynb/go-grpc-compression/lz4"
	_ "github.com/mostynb/go-grpc-compression/zstd"
	_ "github.com/planetscale/vtprotobuf/codec/grpc"
	"github.com/streamingfast/dauth"
	dauthgrpc "github.com/streamingfast/dauth/middleware/grpc"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dgrpc/server/factory"
	"github.com/streamingfast/dgrpc/server/standard"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcv2connect"
	"github.com/streamingfast/substreams/protodecode"
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
	encoding.RegisterCodec(protodecode.PreferredVTCodec{})
}

func ListenTier1(
	listenAddresses []string,
	svc *Tier1Service,
	connectSvc *ConnectService,
	infoService pbsubstreamsrpc.EndpointInfoServer,
	infoServiceConnect pbsubstreamsrpcv2connect.EndpointInfoHandler,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthCheck dgrpcserver.HealthCheck,
	enforceCompression bool,
) (err error) {
	grpcServer := grpcServer(svc, infoService, auth, healthCheck, enforceCompression, logger)
	connectServer := connectServer(connectSvc, infoServiceConnect, auth, healthCheck, enforceCompression, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		logger.Info("handling request", zap.String("content-type", contentType))

		// Loose match for safety
		if strings.HasPrefix(contentType, "application/connect") ||
			strings.HasPrefix(contentType, "application/grpc-web") ||
			contentType == "application/json" ||
			strings.Contains(contentType, "json") {
			logger.Debug("forwarding gRPC-Web request")
			connectServer.ServeHTTP(w, r)
			return
		}

		logger.Debug("forwarding gRPC request")
		grpcServer.ServeHTTP(w, r)
	})

	compressionHandler := standard.CompressionHandler(enforceCompression, mux)

	rootMux := http.NewServeMux()
	rootMux.Handle("/healthz", grpcServer.HealthHandler())
	rootMux.Handle("/", compressionHandler)

	for _, addr := range listenAddresses {
		go func() {
			h2s := &http2.Server{
				MaxConcurrentStreams: 1000,
			}
			handler := h2c.NewHandler(rootMux, h2s)

			if strings.Contains(addr, "*") {
				cleanAddr := strings.ReplaceAll(addr, "*", "")
				tcpListener, err := net.Listen("tcp", cleanAddr)
				if err != nil {
					logger.Error("failed to start secure grpc server", zap.Error(err))
					grpcServer.Shutdown(0)
					return
				}

				errorLogger, err := zap.NewStdLogAt(logger, zap.ErrorLevel)
				if err != nil {
					logger.Error("failed to start secure grpc server", zap.Error(err))
					grpcServer.Shutdown(0)
					return
				}

				httpServer := &http.Server{
					Handler:   handler,
					ErrorLog:  errorLogger,
					TLSConfig: dgrpcserver.SecuredByBuiltInSelfSignedCertificate().AsTLSConfig(),
				}

				logger.Info("serving gRPC (over HTTP router) (secure)", zap.String("listen_addr", addr))
				if err := httpServer.ServeTLS(tcpListener, "", ""); err != nil {
					logger.Error("failed to start secure grpc server", zap.Error(err))
					grpcServer.Shutdown(0)
					return
				}
			} else {
				logger.Info("serving gRPC (over HTTP router) (plain-text)", zap.String("listen_addr", addr))
				if err := http.ListenAndServe(addr, handler); err != nil {
					logger.Error("failed to start secure proxy server", zap.Error(err))
					grpcServer.Shutdown(0)
				}
			}

		}()
	}

	logger.Info("started")

	if grpcServer != nil {
		<-grpcServer.Terminating()
		logger.Info("gRPC server terminated", zap.Error(grpcServer.Error()))
	}
	if connectServer != nil {
		<-connectServer.Terminating()
		logger.Info("gRPC server terminated", zap.Error(connectServer.Error()))
	}

	return
}

func ListenTier2(
	addr string,
	serviceDiscoveryURL *url.URL,
	svc *Tier2Service,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthCheck dgrpcserver.HealthCheck,
	enforceCompression bool,
) (err error) {
	tracerProvider := otel.GetTracerProvider()
	options := []dgrpcserver.Option{

		dgrpcserver.WithLogger(logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, healthCheck),
		dgrpcserver.WithGRPCServerOptions(
			grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracerProvider))),
			grpc.MaxRecvMsgSize(1024*1024*1024),
		),
		// Health probes / LB selectors often open TCP and speak plain HTTP (or drop mid-handshake)
		// against a TLS port; Go's http.Server logs those as ERROR. Suppress the known-benign ones.
		dgrpcserver.WithSuppressHTTPError(
			regexp.MustCompile(`TLS handshake error.*EOF`),
			regexp.MustCompile(`TLS handshake error.*connection reset by peer`),
			regexp.MustCompile(`TLS handshake error.*first record does not look like a TLS handshake`),
			regexp.MustCompile(`http2: server: error reading preface.*connection reset by peer`),
		),
	}
	if enforceCompression {
		options = append(options, dgrpcserver.WithEnforceCompression())
	}

	if serviceDiscoveryURL != nil {
		options = append(options, dgrpcserver.WithServiceDiscoveryURL(serviceDiscoveryURL))
	}
	options = append(options,
		dgrpcserver.WithPostUnaryInterceptor(dauthgrpc.UnaryAuthChecker(auth, logger)),
		dgrpcserver.WithPostStreamInterceptor(dauthgrpc.StreamAuthChecker(auth, logger)),
	)
	if strings.Contains(addr, "*") {
		options = append(options, dgrpcserver.WithInsecureServer())
	} else {
		options = append(options, dgrpcserver.WithPlainTextServer())
	}

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
