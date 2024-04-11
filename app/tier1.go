package app

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/blockstream"
	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	dauth "github.com/streamingfast/dauth"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/service"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type Tier1Modules struct {
	// Required dependencies
	Authenticator         dauth.Authenticator
	HeadTimeDriftMetric   *dmetrics.HeadTimeDrift
	HeadBlockNumberMetric *dmetrics.HeadBlockNum
	CheckPendingShutDown  func() bool
}

type Tier1Config struct {
	MeteringConfig string

	MergedBlocksStoreURL    string
	OneBlocksStoreURL       string
	ForkedBlocksStoreURL    string
	BlockStreamAddr         string        // gRPC endpoint to get real-time blocks, can be "" in which live streams is disabled
	GRPCListenAddr          string        // gRPC address where this app will listen to
	GRPCShutdownGracePeriod time.Duration // The duration we allow for gRPC connections to terminate gracefully prior forcing shutdown
	ServiceDiscoveryURL     *url.URL

	StateStoreURL        string
	StateStoreDefaultTag string
	BlockType            string
	StateBundleSize      uint64

	MaxSubrequests       uint64
	SubrequestsEndpoint  string
	SubrequestsInsecure  bool
	SubrequestsPlaintext bool

	WASMExtensions wasm.WASMExtensioner

	Tracing bool
}

type Tier1App struct {
	*shutter.Shutter
	config  *Tier1Config
	modules *Tier1Modules
	logger  *zap.Logger
	isReady *atomic.Bool
}

func NewTier1(logger *zap.Logger, config *Tier1Config, modules *Tier1Modules) *Tier1App {
	return &Tier1App{
		Shutter: shutter.New(),
		config:  config,
		modules: modules,
		logger:  logger,

		isReady: atomic.NewBool(false),
	}
}

func (a *Tier1App) Run() error {

	dmetrics.Register(metrics.MetricSet)

	a.logger.Info("running substreams-tier1", zap.Reflect("config", a.config))
	if err := a.config.Validate(); err != nil {
		return fmt.Errorf("invalid app config: %w", err)
	}

	mergedBlocksStore, err := dstore.NewDBinStore(a.config.MergedBlocksStoreURL)
	if err != nil {
		return fmt.Errorf("failed setting up block store from url %q: %w", a.config.MergedBlocksStoreURL, err)
	}

	oneBlocksStore, err := dstore.NewDBinStore(a.config.OneBlocksStoreURL)
	if err != nil {
		return fmt.Errorf("failed setting up one-block store from url %q: %w", a.config.OneBlocksStoreURL, err)
	}

	stateStore, err := dstore.NewStore(a.config.StateStoreURL, "zst", "zstd", true)
	if err != nil {
		return fmt.Errorf("failed setting up state store from url %q: %w", a.config.StateStoreURL, err)
	}

	// set to empty store interface if URL is ""
	var forkedBlocksStore dstore.Store
	if a.config.ForkedBlocksStoreURL != "" {
		forkedBlocksStore, err = dstore.NewDBinStore(a.config.ForkedBlocksStoreURL)
		if err != nil {
			return fmt.Errorf("failed setting up block store from url %q: %w", a.config.ForkedBlocksStoreURL, err)
		}
	}

	withLive := a.config.BlockStreamAddr != ""

	var forkableHub *hub.ForkableHub

	if withLive {
		liveSourceFactory := bstream.SourceFactory(func(h bstream.Handler) bstream.Source {
			return blockstream.NewSource(
				context.Background(),
				a.config.BlockStreamAddr,
				2,
				bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
					a.modules.HeadBlockNumberMetric.SetUint64(blk.Number)
					a.modules.HeadTimeDriftMetric.SetBlockTime(blk.Time())
					return h.ProcessBlock(blk, obj)
				}),
				blockstream.WithRequester("substreams-tier1"),
			)
		})

		oneBlocksSourceFactory := bstream.SourceFromNumFactoryWithSkipFunc(func(num uint64, h bstream.Handler, skipFunc func(string) bool) bstream.Source {
			src, err := bstream.NewOneBlocksSource(num, oneBlocksStore, h, bstream.OneBlocksSourceWithSkipperFunc(skipFunc))
			if err != nil {
				return nil
			}
			return src
		})

		forkableHub = hub.NewForkableHub(liveSourceFactory, oneBlocksSourceFactory, 500)
		forkableHub.OnTerminated(a.Shutdown)

		go forkableHub.Run()
	}

	subrequestsClientConfig := client.NewSubstreamsClientConfig(
		a.config.SubrequestsEndpoint,
		"",
		client.None,
		a.config.SubrequestsInsecure,
		a.config.SubrequestsPlaintext,
	)
	var opts []service.Option
	if a.config.WASMExtensions != nil {
		opts = append(opts, service.WithWASMExtensioner(a.config.WASMExtensions))
	}

	if a.config.Tracing {
		opts = append(opts, service.WithModuleExecutionTracing())
	}

	var wasmModules map[string]string
	if a.config.WASMExtensions != nil {
		wasmModules = a.config.WASMExtensions.Params()
	}

	tier2RequestParameters := reqctx.Tier2RequestParameters{
		MeteringConfig:       a.config.MeteringConfig,
		FirstStreamableBlock: bstream.GetProtocolFirstStreamableBlock,
		MergedBlockStoreURL:  a.config.MergedBlocksStoreURL,
		StateStoreURL:        a.config.StateStoreURL,
		StateBundleSize:      a.config.StateBundleSize,
		StateStoreDefaultTag: a.config.StateStoreDefaultTag,
		WASMModules:          wasmModules,
	}

	svc, err := service.NewTier1(
		a.logger,
		mergedBlocksStore,
		forkedBlocksStore,
		forkableHub,
		stateStore,
		a.config.StateStoreDefaultTag,
		a.config.MaxSubrequests,
		a.config.StateBundleSize,
		a.config.BlockType,
		subrequestsClientConfig,
		tier2RequestParameters,
		opts...,
	)
	if err != nil {
		return err
	}

	a.OnTerminating(func(err error) {
		metrics.AppReadinessTier1.SetNotReady()

		svc.Shutdown(err)
		time.Sleep(2 * time.Second) // enough time to send termination grpc responses
	})

	go func() {
		if withLive {
			a.logger.Info("waiting until hub is real-time synced")
			select {
			case <-forkableHub.Ready:
				metrics.AppReadinessTier1.SetReady()
			case <-a.Terminating():
				return
			}
		}

		a.logger.Info("launching gRPC server", zap.Bool("live_support", withLive))
		a.isReady.CompareAndSwap(false, true)

		err := service.ListenTier1(a.config.GRPCListenAddr, svc, a.modules.Authenticator, a.logger, a.HealthCheck)
		a.Shutdown(err)
	}()

	return nil
}

func (a *Tier1App) HealthCheck(ctx context.Context) (bool, interface{}, error) {
	return a.IsReady(ctx), nil, nil
}

// IsReady return `true` if the apps is ready to accept requests, `false` is returned
// otherwise.
func (a *Tier1App) IsReady(ctx context.Context) bool {
	if a.IsTerminating() {
		return false
	}
	if !a.modules.Authenticator.Ready(ctx) {
		return false
	}

	if a.modules.CheckPendingShutDown != nil && a.modules.CheckPendingShutDown() {
		return false
	}

	return a.isReady.Load()
}

// Validate inspects itself to determine if the current config is valid according to
// substreams rules.
func (config *Tier1Config) Validate() error {
	return nil
}
