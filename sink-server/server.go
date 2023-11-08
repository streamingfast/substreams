package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	connect_go "github.com/bufbuild/connect-go"
	"github.com/google/uuid"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	connectweb "github.com/streamingfast/dgrpc/server/connect-web"
	"github.com/streamingfast/shutter"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
	docker "github.com/streamingfast/substreams/sink-server/docker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const DeploymentIDLength = 8

type server struct {
	*shutter.Shutter
	// provider Provider

	httpListenAddr     string
	corsHostRegexAllow *regexp.Regexp

	logger *zap.Logger
	engine Engine
}

func New(
	ctx context.Context,
	engine string,
	dataDir string,
	httpListenAddr string,
	corsHostRegexAllow *regexp.Regexp,
	logger *zap.Logger,
) (*server, error) {

	srv := &server{
		Shutter:            shutter.New(),
		httpListenAddr:     httpListenAddr,
		corsHostRegexAllow: corsHostRegexAllow,
		logger:             logger,
	}

	token := os.Getenv("SUBSTREAMS_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("error: please set SUBSTREAMS_API_TOKEN environment variable to a valid substreams API token")
	}

	switch engine {
	case "docker":
		engine, err := docker.NewEngine(dataDir, token)
		if err != nil {
			return nil, err
		}
		srv.engine = engine
	default:
		return nil, fmt.Errorf("unsupported engine: %q", engine)
	}

	return srv, nil
}

// this is a blocking call
func (s *server) Run() {
	s.logger.Info("starting server server")

	tracerProvider := otel.GetTracerProvider()
	options := []dgrpcserver.Option{
		dgrpcserver.WithLogger(s.logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, s.healthzHandler()),
		dgrpcserver.WithPostUnaryInterceptor(otelgrpc.UnaryServerInterceptor(otelgrpc.WithTracerProvider(tracerProvider))),
		dgrpcserver.WithPostStreamInterceptor(otelgrpc.StreamServerInterceptor(otelgrpc.WithTracerProvider(tracerProvider))),
		dgrpcserver.WithGRPCServerOptions(grpc.MaxRecvMsgSize(25 * 1024 * 1024)),
		dgrpcserver.WithReflection(pbsinksvc.Provider_ServiceDesc.ServiceName),
		dgrpcserver.WithCORS(s.corsOption()),
	}
	if strings.Contains(s.httpListenAddr, "*") {
		s.logger.Info("grpc server with insecure server")
		options = append(options, dgrpcserver.WithInsecureServer())
	} else {
		s.logger.Info("grpc server with plain text server")
		options = append(options, dgrpcserver.WithPlainTextServer())
	}

	streamHandlerGetter := func(opts ...connect_go.HandlerOption) (string, http.Handler) {
		return pbsinksvcconnect.NewProviderHandler(s, opts...)
	}

	srv := connectweb.New([]connectweb.HandlerGetter{streamHandlerGetter}, options...)
	addr := strings.ReplaceAll(s.httpListenAddr, "*", "")

	s.OnTerminating(func(err error) {
		time.Sleep(time.Second)
		s.logger.Info("shutting down connect web server")
		srv.Shutdown(nil)
		s.engine.Shutdown(s.logger)
	})

	srv.Launch(addr)
	<-srv.Terminated()
}

func genDeployID() string {
	return uuid.New().String()[0:DeploymentIDLength]
}

func (s *server) Deploy(ctx context.Context, req *connect_go.Request[pbsinksvc.DeployRequest]) (*connect_go.Response[pbsinksvc.DeployResponse], error) {
	id := genDeployID()

	s.logger.Info("deployment request", zap.String("deployment_id", id))

	err := s.engine.Create(id, req.Msg.SubstreamsPackage, s.logger)
	if err != nil {
		return nil, err
	}

	status, reason, outs, _, _, err := s.engine.Info(id, s.logger)
	if err != nil {
		return nil, err
	}

	return connect_go.NewResponse(&pbsinksvc.DeployResponse{
		Status:       status,
		Reason:       reason,
		DeploymentId: id,
		Services:     outs,
	}), nil
}

func (s *server) Update(ctx context.Context, req *connect_go.Request[pbsinksvc.UpdateRequest]) (*connect_go.Response[pbsinksvc.UpdateResponse], error) {
	id := req.Msg.DeploymentId
	_, _, _, _, _, err := s.engine.Info(id, s.logger) // only checking if it exists
	if err != nil {
		return nil, fmt.Errorf("looking up deployment %q: %w", id, err)
	}

	s.logger.Info("update request", zap.String("deployment_id", id))

	err = s.engine.Update(id, req.Msg.SubstreamsPackage, req.Msg.Reset_, s.logger)
	if err != nil {
		return nil, err
	}

	status, reason, outs, _, _, err := s.engine.Info(id, s.logger)
	if err != nil {
		return nil, err
	}

	return connect_go.NewResponse(&pbsinksvc.UpdateResponse{
		Status:   status,
		Reason:   reason,
		Services: outs,
	}), nil
}

func (s *server) Info(ctx context.Context, req *connect_go.Request[pbsinksvc.InfoRequest]) (*connect_go.Response[pbsinksvc.InfoResponse], error) {
	status, reason, outs, info, prog, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, err
	}

	return connect_go.NewResponse(
		&pbsinksvc.InfoResponse{
			Status:      status,
			Reason:      reason,
			Services:    outs,
			PackageInfo: info,
			Progress:    prog,
		}), nil
}

func (s *server) List(ctx context.Context, req *connect_go.Request[pbsinksvc.ListRequest]) (*connect_go.Response[pbsinksvc.ListResponse], error) {
	s.logger.Info("list request")

	list, err := s.engine.List(s.logger)
	if err != nil {
		return nil, fmt.Errorf("listing: %w", err)
	}

	out := &pbsinksvc.ListResponse{}
	for _, d := range list {
		out.Deployments = append(out.Deployments, &pbsinksvc.DeploymentWithStatus{
			Id:          d.Id,
			Status:      d.Status,
			Reason:      d.Reason,
			PackageInfo: d.PackageInfo,
			Progress:    d.Progress,
		})
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Pause(ctx context.Context, req *connect_go.Request[pbsinksvc.PauseRequest]) (*connect_go.Response[pbsinksvc.PauseResponse], error) {
	s.logger.Info("pause request", zap.String("deployment_id", req.Msg.DeploymentId))

	prevState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	_, err = s.engine.Pause(req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, fmt.Errorf("pausing %q: %w", req.Msg.DeploymentId, err)
	}

	newState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get new status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	out := &pbsinksvc.PauseResponse{
		PreviousStatus: prevState,
		NewStatus:      newState,
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Stop(ctx context.Context, req *connect_go.Request[pbsinksvc.StopRequest]) (*connect_go.Response[pbsinksvc.StopResponse], error) {
	s.logger.Info("pause request", zap.String("deployment_id", req.Msg.DeploymentId))

	prevState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	_, err = s.engine.Stop(req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, fmt.Errorf("stopping %q: %w", req.Msg.DeploymentId, err)
	}

	newState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get new status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	out := &pbsinksvc.StopResponse{
		PreviousStatus: prevState,
		NewStatus:      newState,
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Resume(ctx context.Context, req *connect_go.Request[pbsinksvc.ResumeRequest]) (*connect_go.Response[pbsinksvc.ResumeResponse], error) {
	s.logger.Info("resume request", zap.String("deployment_id", req.Msg.DeploymentId))

	prevState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	_, err = s.engine.Resume(req.Msg.DeploymentId, prevState, s.logger)
	if err != nil {
		return nil, fmt.Errorf("resuming %q: %w", req.Msg.DeploymentId, err)
	}

	newState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get new status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	out := &pbsinksvc.ResumeResponse{
		PreviousStatus: prevState,
		NewStatus:      newState,
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Remove(ctx context.Context, req *connect_go.Request[pbsinksvc.RemoveRequest]) (*connect_go.Response[pbsinksvc.RemoveResponse], error) {
	s.logger.Info("remove request", zap.String("deployment_id", req.Msg.DeploymentId))

	prevState, _, _, _, _, err := s.engine.Info(req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}

	_, err = s.engine.Remove(req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, fmt.Errorf("removing %q: %w", req.Msg.DeploymentId, err)
	}

	out := &pbsinksvc.RemoveResponse{
		PreviousStatus: prevState,
	}
	return connect_go.NewResponse(out), nil
}
