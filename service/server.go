package service

import (
	"net/url"
	"strings"
	"time"

	_ "github.com/mostynb/go-grpc-compression/experimental/s2"
	_ "github.com/mostynb/go-grpc-compression/lz4"
	_ "github.com/mostynb/go-grpc-compression/zstd"
	"github.com/streamingfast/dauth"
	dauthgrpc "github.com/streamingfast/dauth/middleware/grpc"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dgrpc/server/factory"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/experimental"
)

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

type v3Adapter struct {
	*Tier1Service
}

func (a *v3Adapter) Blocks(req *pbsubstreamsrpcv3.Request, srv pbsubstreamsrpcv3.Stream_BlocksServer) error {
	return a.BlocksV3(req, srv)
}

func ListenTier1(
	listenAddr string,
	svc *Tier1Service,
	infoService pbsubstreamsrpc.EndpointInfoServer,
	auth dauth.Authenticator,
	logger *zap.Logger,
	healthcheck dgrpcserver.HealthCheck,
	enforceCompression bool,
) (err error) {

	done := make(chan any)

	var servers []dgrpcserver.Server
	for _, addr := range strings.Split(listenAddr, ",") {
		// note: some of these common options don't work with connectWeb
		options := GetCommonServerOptions(addr, logger, healthcheck, enforceCompression)
		options = append(options, dgrpcserver.WithPostUnaryInterceptor(dauthgrpc.UnaryAuthChecker(auth, logger)))
		options = append(options, dgrpcserver.WithPostStreamInterceptor(dauthgrpc.StreamAuthChecker(auth, logger)))
		grpcServer := factory.ServerFromOptions(options...)
		pbsubstreamsrpc.RegisterStreamServer(grpcServer.ServiceRegistrar(), svc)
		pbsubstreamsrpcv3.RegisterStreamServer(grpcServer.ServiceRegistrar(), &v3Adapter{svc})
		if infoService != nil {
			pbsubstreamsrpc.RegisterEndpointInfoServer(grpcServer.ServiceRegistrar(), infoService)
		}
		svc.OnTerminating(func(err error) {
			logger.Info("Tier1Service is terminating")
			grpcServer.Shutdown(30 * time.Second)
		})
		cleanAddr := strings.ReplaceAll(addr, "*", "")
		servers = append(servers, grpcServer)
		go func() {
			grpcServer.Launch(cleanAddr)
			close(done)
		}()
	}

	<-done
	for _, srv := range servers {
		srv.Shutdown(0)
	}

	//GRRRRRRRRRRRRRR
	//GRRRRRRRRRRRRRR

	//for _, srv := range servers {
	//	<-srv.Terminated()
	//	if e := srv.Err(); e != nil {
	//		err = e
	//	}
	//}
	//GRRRRRRRRRRRRRR
	//GRRRRRRRRRRRRRR

	return
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
		grpcServer.Shutdown(30 * time.Second)
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
