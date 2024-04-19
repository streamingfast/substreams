package app

import (
	"context"
	"fmt"
	"net/url"

	dauth "github.com/streamingfast/dauth"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/service"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type Tier2Config struct {
	GRPCListenAddr      string // gRPC address where this app will listen to
	ServiceDiscoveryURL *url.URL

	PipelineOptions []pipeline.Option

	MaximumConcurrentRequests uint64
	WASMExtensions            wasm.WASMExtensioner

	Tracing bool
}

type Tier2App struct {
	*shutter.Shutter
	config  *Tier2Config
	modules *Tier2Modules
	logger  *zap.Logger
	isReady *atomic.Bool
}

type Tier2Modules struct {
	CheckPendingShutDown func() bool
}

func NewTier2(logger *zap.Logger, config *Tier2Config, modules *Tier2Modules) *Tier2App {
	return &Tier2App{
		Shutter: shutter.New(),
		config:  config,
		modules: modules,
		logger:  logger,

		isReady: atomic.NewBool(false),
	}
}

func (a *Tier2App) Run() error {
	dmetrics.Register(metrics.MetricSet)

	a.logger.Info("running substreams-tier2", zap.Reflect("config", a.config))
	if err := a.config.Validate(); err != nil {
		return fmt.Errorf("invalid app config: %w", err)
	}

	var opts []service.Option
	//for _, opt := range a.config.PipelineOptions {
	//	opts = append(opts, service.WithPipelineOptions(opt))
	//}

	if a.config.Tracing {
		opts = append(opts, service.WithModuleExecutionTracing())
	}

	if a.config.MaximumConcurrentRequests > 0 {
		opts = append(opts, service.WithMaxConcurrentRequests(a.config.MaximumConcurrentRequests))
	}
	opts = append(opts, service.WithReadinessFunc(a.setReadiness))

	if a.config.WASMExtensions != nil {
		opts = append(opts, service.WithWASMExtensioner(a.config.WASMExtensions))
	}

	svc, err := service.NewTier2(
		a.logger,
		opts...,
	)
	if err != nil {
		return err
	}

	// tier2 always trusts the headers sent from tier1
	trustAuth, err := dauth.New("trust://", a.logger)
	if err != nil {
		return fmt.Errorf("failed to setup trust authenticator: %w", err)
	}

	a.OnTerminating(func(_ error) { metrics.AppReadinessTier2.SetNotReady() })

	go func() {
		a.logger.Info("launching gRPC server")
		a.isReady.CompareAndSwap(false, true)
		metrics.AppReadinessTier2.SetReady()

		err := service.ListenTier2(a.config.GRPCListenAddr, a.config.ServiceDiscoveryURL, svc, trustAuth, a.logger, a.HealthCheck)
		a.Shutdown(err)
	}()

	return nil
}

func (a *Tier2App) HealthCheck(ctx context.Context) (bool, interface{}, error) {
	return a.IsReady(ctx), nil, nil
}

// IsReady return `true` if the apps is ready to accept requests, `false` is returned
// otherwise.
func (a *Tier2App) IsReady(ctx context.Context) bool {
	if a.IsTerminating() {
		return false
	}

	if a.modules.CheckPendingShutDown != nil && a.modules.CheckPendingShutDown() {
		return false
	}

	return a.isReady.Load()
}

func (a *Tier2App) setReadiness(ready bool) {
	a.isReady.Store(ready)
}

// Validate inspects itself to determine if the current config is valid according to
// substreams rules.
func (config *Tier2Config) Validate() error {
	return nil
}
