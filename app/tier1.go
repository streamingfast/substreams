package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/blockstream"
	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	dauth "github.com/streamingfast/dauth"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dsession"
	"github.com/streamingfast/dstore"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v2"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service"
	"github.com/streamingfast/substreams/service/connect"
	"github.com/streamingfast/substreams/wasm"

	_ "github.com/streamingfast/substreams/wasm/wasmtime"
	"github.com/streamingfast/substreams/wasm/wazero"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type Tier1Modules struct {
	// Required dependencies
	Authenticator         dauth.Authenticator
	SessionPool           dsession.SessionPool
	HeadTimeDriftMetric   *dmetrics.HeadTimeDrift
	HeadBlockNumberMetric *dmetrics.HeadBlockNum
	CheckPendingShutDown  func() bool
	InfoServer            InfoServer
}

type InfoServer interface {
	Init(ctx context.Context, fhub *hub.ForkableHub, mergedBlocksStore dstore.Store, oneBlockStore dstore.Store, logger *zap.Logger) error
	Info(ctx context.Context, request *pbfirehose.InfoRequest) (*pbfirehose.InfoResponse, error)
}

// returns config with default sane values
func NewDefaultTier1Config() *Tier1Config {
	return &Tier1Config{
		SharedCacheSize:       15,
		MaxSubrequests:        10,
		StateBundleSize:       1000,
		BlockExecutionTimeout: 1 * time.Minute,
	}
}

type Tier1Config struct {
	MeteringConfig string

	FoundationalStoresConfigPath string

	MergedBlocksStoreURL    string
	OneBlocksStoreURL       string
	ForkedBlocksStoreURL    string
	BlockStreamAddr         string        // gRPC endpoint to get real-time blocks, can be "" in which live streams is disabled
	GRPCListenAddr          string        // gRPC address where this app will listen to
	GRPCShutdownGracePeriod time.Duration // The duration we allow for gRPC connections to terminate gracefully prior forcing shutdown
	ServiceDiscoveryURL     *url.URL
	BlockExecutionTimeout   time.Duration

	TmpDir                  string
	StateStoreURL           string
	QuickSaveStoreURL       string
	StateStoreDefaultTag    string
	BlockType               string
	StateBundleSize         uint64
	EnforceCompression      bool // refuse incoming requests that do not accept gzip compression (ConnectRPC or GRPC)
	ActiveRequestsSoftLimit int  // maximum number of active requests a tier1 app can have with external clients before starting to advertise itself as unready in the health check

	ActiveRequestsHardLimit int // maximum number of active requests a tier1 app can have with external clients, refuse with CodeUnavailable if reached
	MaxSubrequests          uint64
	SubrequestsEndpoint     string
	SubrequestsInsecure     bool
	SubrequestsPlaintext    bool

	SharedCacheSize uint64

	WASMExtensions wasm.WASMExtensioner
	Tracing        bool
}

type Tier1App struct {
	*shutter.Shutter
	config  *Tier1Config
	modules *Tier1Modules
	logger  *zap.Logger
	isReady *atomic.Bool
}

func NewTier1(logger *zap.Logger, config *Tier1Config, modules *Tier1Modules) *Tier1App {
	if modules.CheckPendingShutDown == nil {
		modules.CheckPendingShutDown = func() bool { return false }
	}

	metrics.DeclareTier1Metrics(logger)

	return &Tier1App{
		Shutter: shutter.New(),
		config:  config,
		modules: modules,
		logger:  logger,

		isReady: atomic.NewBool(false),
	}
}

func loadTier1FoundationalStoreEndpoints(configPath string) (map[string]string, error) {
	if configPath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read foundational stores config file %s: %w", configPath, err)
	}

	var endpoints map[string]string
	if err := json.Unmarshal(data, &endpoints); err != nil {
		return nil, fmt.Errorf("failed to parse foundational stores config file %s: %w", configPath, err)
	}

	return endpoints, nil
}

func (a *Tier1App) Run() error {
	// declared in NewTier1, registered here
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

	var quickSaveStore dstore.Store
	if a.config.QuickSaveStoreURL != "" {
		quickSaveStore, err = dstore.NewStore(a.config.QuickSaveStoreURL, "zst", "zstd", true)
		if err != nil {
			return fmt.Errorf("failed setting up quickSave store from url %q: %w", a.config.QuickSaveStoreURL, err)
		}
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

		forkableHub = hub.NewForkableHub(liveSourceFactory, 200, oneBlocksStore)
		forkableHub.OnTerminated(a.Shutdown)

		go forkableHub.Run()
	}

	subRequestsClientConfig := client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
		Endpoint:             a.config.SubrequestsEndpoint,
		AuthToken:            "",
		AuthType:             client.None,
		Insecure:             a.config.SubrequestsInsecure,
		PlainText:            a.config.SubrequestsPlaintext,
		Agent:                "substreams_tier1",
		ForceProtocolVersion: client.ProtocolVersionUnset, // unused for tier2 requests
	})
	var opts []service.Option
	if a.config.WASMExtensions != nil {
		opts = append(opts, service.WithWASMExtensioner(a.config.WASMExtensions))
	}

	if a.config.Tracing {
		opts = append(opts, service.WithModuleExecutionTracing())
	}

	if a.config.BlockExecutionTimeout != 0 {
		opts = append(opts, service.WithBlockExecutionTimeout(a.config.BlockExecutionTimeout))
	}

	if a.config.TmpDir != "" {
		wazero.SetTempDir(a.config.TmpDir)
	}

	foundationalStoreEndpoints, err := loadTier1FoundationalStoreEndpoints(a.config.FoundationalStoresConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load foundational store endpoints %q: %w", a.config.FoundationalStoresConfigPath, err)
	}

	var wasmModules map[string]string
	if a.config.WASMExtensions != nil {
		wasmModules = a.config.WASMExtensions.Params()
	}

	tier2RequestParameters := reqctx.Tier2RequestParameters{
		MeteringConfig:             a.config.MeteringConfig,
		FirstStreamableBlock:       bstream.GetProtocolFirstStreamableBlock,
		MergedBlockStoreURL:        a.config.MergedBlocksStoreURL,
		StateStoreURL:              a.config.StateStoreURL,
		StateBundleSize:            a.config.StateBundleSize,
		StateStoreDefaultTag:       a.config.StateStoreDefaultTag,
		WASMModules:                wasmModules,
		FoundationalStoreEndpoints: foundationalStoreEndpoints,
	}

	tier1Service, err := service.NewTier1(
		a.logger,
		mergedBlocksStore,
		forkedBlocksStore,
		forkableHub,
		stateStore,
		quickSaveStore,
		a.config.StateStoreDefaultTag,
		a.config.MaxSubrequests,
		a.config.StateBundleSize,
		a.config.BlockType,
		a.setIsReady,
		subRequestsClientConfig,
		tier2RequestParameters,
		a.config.EnforceCompression,
		a.config.ActiveRequestsSoftLimit,
		a.config.ActiveRequestsHardLimit,
		a.config.SharedCacheSize,
		a.modules.SessionPool,
		foundationalStoreEndpoints,
		opts...,
	)
	if err != nil {
		return err
	}

	tier1ServiceConnect := connect.NewService(tier1Service)

	a.OnTerminating(func(err error) {
		metrics.AppReadinessTier1.SetNotReady()

		tier1Service.Shutdown(err)
	})

	go func() {
		var infoServer pbsubstreamsrpcv2.EndpointInfoServer
		if a.modules.InfoServer != nil {
			a.logger.Info("waiting until info server is ready")
			infoServer = &InfoServerWrapper{a.modules.InfoServer}
			if err := a.modules.InfoServer.Init(context.Background(), forkableHub, mergedBlocksStore, oneBlocksStore, a.logger); err != nil {
				a.Shutdown(fmt.Errorf("cannot initialize info server: %w", err))
				return
			}
		}

		if withLive {
			a.logger.Info("waiting until hub is real-time synced")
			select {
			case <-forkableHub.Ready:
				// Wait until the hub is ready
			case <-a.Terminating():
				return
			}
		}

		a.logger.Info("launching gRPC server", zap.Bool("live_support", withLive))
		a.setIsReady(true)

		secureGrpcProxyAddr := ":9000"
		plaintextGrpcProxyAddr := ":8080"

		addresses := strings.Split(a.config.GRPCListenAddr, ",")
		addressCount := len(addresses)
		if addressCount == 0 {
			a.logger.Error("no gRPC listen addresses provided")
			return
		}
		if addressCount > 0 {
			secureGrpcProxyAddr = addresses[0]
		}
		if addressCount > 1 {
			plaintextGrpcProxyAddr = addresses[1]
		}

		err := service.ListenTier1(secureGrpcProxyAddr, plaintextGrpcProxyAddr, tier1Service, tier1ServiceConnect, infoServer, a.modules.Authenticator, a.logger, a.HealthCheck, a.config.EnforceCompression)
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

func (a *Tier1App) setIsReady(ready bool) {
	if ready {
		a.isReady.Store(true)
		metrics.AppReadinessTier1.SetReady()
	} else {
		a.isReady.Store(false)
		metrics.AppReadinessTier1.SetNotReady()
	}
}

// Validate inspects itself to determine if the current config is valid according to
// substreams rules.
func (config *Tier1Config) Validate() error {
	return nil
}

var _ pbsubstreamsrpcv2.EndpointInfoServer = (*InfoServerWrapper)(nil)

type InfoServerWrapper struct {
	rpcInfoServer pbsubstreamsrpcv2.EndpointInfoServer
}

// Info implements pbsubstreamsrpcconnect.EndpointInfoHandler.
func (i *InfoServerWrapper) Info(ctx context.Context, req *pbfirehose.InfoRequest) (*pbfirehose.InfoResponse, error) {
	resp, err := i.rpcInfoServer.Info(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
