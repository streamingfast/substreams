package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	connect_go "connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/streamingfast/dauth"
	dauthconnect "github.com/streamingfast/dauth/middleware/connect"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	connectweb "github.com/streamingfast/dgrpc/server/connectrpc"
	"github.com/streamingfast/shutter"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
	sinkcontext "github.com/streamingfast/substreams/sink-server/context"
	"go.uber.org/zap"
)

const DeploymentIDLength = 8

type server struct {
	*shutter.Shutter
	// provider Provider

	httpListenAddr     string
	corsHostRegexAllow *regexp.Regexp

	authenticator dauth.Authenticator
	logger        *zap.Logger
	engine        Engine

	shutdownLock sync.RWMutex
}

func New(
	ctx context.Context,
	engine Engine,
	dataDir string,
	httpListenAddr string,
	corsHostRegexAllow *regexp.Regexp,
	authenticator dauth.Authenticator,
	logger *zap.Logger,
) (*server, error) {
	srv := &server{
		Shutter:            shutter.New(),
		httpListenAddr:     httpListenAddr,
		corsHostRegexAllow: corsHostRegexAllow,
		authenticator:      authenticator,
		logger:             logger,
		engine:             engine,
	}

	return srv, nil
}

// this is a blocking call
func (s *server) Run(ctx context.Context) {
	s.logger.Debug("starting server")

	options := []dgrpcserver.Option{
		dgrpcserver.WithLogger(s.logger),
		dgrpcserver.WithHealthCheck(dgrpcserver.HealthCheckOverGRPC|dgrpcserver.HealthCheckOverHTTP, s.healthzHandler()),
		dgrpcserver.WithConnectInterceptor(dauthconnect.NewAuthInterceptor(s.authenticator, s.logger)),
		dgrpcserver.WithReflection(pbsinksvc.Provider_ServiceDesc.ServiceName),
		dgrpcserver.WithCORS(s.corsOption()),
	}
	if strings.Contains(s.httpListenAddr, "*") {
		s.logger.Warn("grpc server with insecure server")
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
		s.shutdownLock.Lock()
		s.logger.Warn("shutting down connect web server")

		shutdownErr := s.engine.Shutdown(ctx, err, s.logger)
		if shutdownErr != nil {
			s.logger.Warn("failed to shutdown engine", zap.Error(shutdownErr))
		}

		time.Sleep(1 * time.Second)

		srv.Shutdown(nil)
		s.logger.Warn("connect web server shutdown")
	})

	s.OnTerminated(func(err error) {
		s.shutdownLock.Unlock()
	})

	srv.Launch(addr)
	<-srv.Terminated()
}

func genDeployID(uid string) string {
	return uid[0:DeploymentIDLength]
}

func (s *server) Deploy(ctx context.Context, req *connect_go.Request[pbsinksvc.DeployRequest]) (*connect_go.Response[pbsinksvc.DeployResponse], error) {
	s.shutdownLock.RLock()
	defer s.shutdownLock.RUnlock()

	uid := uuid.New().String()
	id := genDeployID(uid)

	ctx = sinkcontext.SetHeader(ctx, req.Header())
	ctx = sinkcontext.SetProductionMode(ctx, !req.Msg.GetDevelopmentMode())
	paramMap := map[string]string{}
	for _, param := range req.Msg.GetParameters() {
		paramMap[param.Key] = param.Value
	}
	ctx = sinkcontext.SetParameterMap(ctx, paramMap)

	s.logger.Info("deployment request", zap.String("deployment_id", id))

	info, err := s.engine.Create(ctx, id, req.Msg.SubstreamsPackage, s.logger)
	if err != nil {
		return nil, err
	}

	return connect_go.NewResponse(&pbsinksvc.DeployResponse{
		Status:       info.Status,
		Reason:       info.Reason,
		DeploymentId: id,
		Services:     info.Services,
		Motd:         info.Motd,
	}), nil
}

func (s *server) Update(ctx context.Context, req *connect_go.Request[pbsinksvc.UpdateRequest]) (*connect_go.Response[pbsinksvc.UpdateResponse], error) {
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	id := req.Msg.DeploymentId
	_, err := s.engine.Info(ctx, id, s.logger) // only checking if it exists
	if err != nil {
		return nil, fmt.Errorf("looking up deployment %q: %w", id, err)
	}

	s.logger.Info("update request", zap.String("deployment_id", id))

	err = s.engine.Update(ctx, id, req.Msg.SubstreamsPackage, req.Msg.Reset_, s.logger)
	if err != nil {
		return nil, err
	}

	info, err := s.engine.Info(ctx, id, s.logger)
	if err != nil {
		return nil, err
	}

	return connect_go.NewResponse(&pbsinksvc.UpdateResponse{
		Status:   info.Status,
		Reason:   info.Reason,
		Services: info.Services,
		Motd:     info.Motd,
	}), nil
}

func (s *server) Info(ctx context.Context, req *connect_go.Request[pbsinksvc.InfoRequest]) (*connect_go.Response[pbsinksvc.InfoResponse], error) {
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	info, err := s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, err
	}

	return connect_go.NewResponse(info), nil
}

func (s *server) List(ctx context.Context, req *connect_go.Request[pbsinksvc.ListRequest]) (*connect_go.Response[pbsinksvc.ListResponse], error) {
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	s.logger.Info("list request")

	list, err := s.engine.List(ctx, s.logger)
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
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	s.logger.Info("pause request", zap.String("deployment_id", req.Msg.DeploymentId))

	info, err := s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}
	prevState := info.Status

	_, err = s.engine.Pause(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, fmt.Errorf("pausing %q: %w", req.Msg.DeploymentId, err)
	}

	info, err = s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get new status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}
	newState := info.Status
	if newState == pbsinksvc.DeploymentStatus_UNKNOWN || newState == pbsinksvc.DeploymentStatus_RUNNING {
		newState = pbsinksvc.DeploymentStatus_PAUSING
	}

	out := &pbsinksvc.PauseResponse{
		PreviousStatus: prevState,
		NewStatus:      newState,
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Stop(ctx context.Context, req *connect_go.Request[pbsinksvc.StopRequest]) (*connect_go.Response[pbsinksvc.StopResponse], error) {
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	s.logger.Info("pause request", zap.String("deployment_id", req.Msg.DeploymentId))

	info, err := s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}
	prevState := info.Status

	_, err = s.engine.Stop(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, fmt.Errorf("stopping %q: %w", req.Msg.DeploymentId, err)
	}

	info, err = s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get new status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}
	newState := info.Status
	if newState == pbsinksvc.DeploymentStatus_UNKNOWN || newState == pbsinksvc.DeploymentStatus_RUNNING {
		newState = pbsinksvc.DeploymentStatus_STOPPING
	}

	out := &pbsinksvc.StopResponse{
		PreviousStatus: prevState,
		NewStatus:      newState,
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Resume(ctx context.Context, req *connect_go.Request[pbsinksvc.ResumeRequest]) (*connect_go.Response[pbsinksvc.ResumeResponse], error) {
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	s.logger.Info("resume request", zap.String("deployment_id", req.Msg.DeploymentId))

	info, err := s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get previous status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}
	prevState := info.Status

	_, err = s.engine.Resume(ctx, req.Msg.DeploymentId, prevState, s.logger)
	if err != nil {
		return nil, fmt.Errorf("resuming %q: %w", req.Msg.DeploymentId, err)
	}

	info, err = s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		s.logger.Warn("cannot get new status on deployment", zap.Error(err), zap.String("deployent_id", req.Msg.DeploymentId))
	}
	newState := info.Status
	if newState == pbsinksvc.DeploymentStatus_UNKNOWN || newState == pbsinksvc.DeploymentStatus_PAUSED {
		newState = pbsinksvc.DeploymentStatus_RESUMING
	}

	out := &pbsinksvc.ResumeResponse{
		PreviousStatus: prevState,
		NewStatus:      newState,
	}
	return connect_go.NewResponse(out), nil
}

func (s *server) Remove(ctx context.Context, req *connect_go.Request[pbsinksvc.RemoveRequest]) (*connect_go.Response[pbsinksvc.RemoveResponse], error) {
	ctx = sinkcontext.SetHeader(ctx, req.Header())
	s.logger.Info("remove request", zap.String("deployment_id", req.Msg.DeploymentId))

	info, err := s.engine.Info(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, err
	}
	prevState := info.Status

	_, err = s.engine.Remove(ctx, req.Msg.DeploymentId, s.logger)
	if err != nil {
		return nil, fmt.Errorf("removing %q: %w", req.Msg.DeploymentId, err)
	}

	out := &pbsinksvc.RemoveResponse{
		PreviousStatus: prevState,
	}
	return connect_go.NewResponse(out), nil
}
