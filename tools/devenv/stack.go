// Package devenv boots a complete local Substreams stack — a dummy blockchain in a container,
// plus tier1 and tier2 running in-process — against which requests can be issued.
//
// The same setup backs both the end-to-end tests and `substreams tools devenv`, so what a
// developer watches locally is the code path CI exercises.
//
// It lives here rather than in firehose-core — the natural home, since that is where the
// reader, merger and relayer come from — because firehose-core depends on substreams, so a
// command over there could only ever run the substreams version it pins, never the working
// tree. Tier1 and tier2 are built in this module, so this side of the dependency edge is the
// only one that can run your changes.
package devenv

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/dauth"
	dauthnull "github.com/streamingfast/dauth/null"
	dauthsecret "github.com/streamingfast/dauth/secret"
	dauthtrust "github.com/streamingfast/dauth/trust"
	meteringlogger "github.com/streamingfast/dmetering/logger"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dsession"
	// Registers the "local" session pool plugin StartTier1 asks for.
	_ "github.com/streamingfast/dsession/local"
	"github.com/streamingfast/substreams/app"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

// BlockType is the only chain this stack knows about: the dummy blockchain used across the
// end-to-end tests.
const BlockType = "sf.acme.type.v1.Block"

// DefaultImage is the dummy blockchain image the end-to-end tests are pinned to.
const DefaultImage = "ghcr.io/streamingfast/dummy-blockchain:1cea671"

// ChainConfig describes the dummy blockchain to run.
type ChainConfig struct {
	// Image defaults to DefaultImage.
	Image string
	// TmpDir is bind-mounted as the firehose storage directory, and is also where tier1 keeps
	// its merged blocks and its state store. Wiping it between runs is what forces a cold
	// backprocess.
	TmpDir string
	// Burst is the number of blocks produced immediately at genesis. This is the knob that
	// decides how much there is to backprocess.
	Burst int
	// BlockRate is the number of blocks per minute produced after the burst.
	BlockRate int
	// ExtraReaderArgs are appended to the reader node arguments verbatim.
	ExtraReaderArgs string
	// StartupTimeout bounds the wait for the container to serve. A large Burst pushes this
	// out — the node produces every genesis block before it starts serving — so it scales
	// with the burst when left at zero.
	StartupTimeout time.Duration
}

func (c ChainConfig) startupTimeout() time.Duration {
	if c.StartupTimeout != 0 {
		return c.StartupTimeout
	}

	timeout := 30*time.Second + time.Duration(c.Burst/1000)*10*time.Second
	if timeout > 10*time.Minute {
		return 10 * time.Minute
	}
	return timeout
}

func (c ChainConfig) image() string {
	if c.Image == "" {
		return DefaultImage
	}
	return c.Image
}

func (c ChainConfig) readerArgs() string {
	blockRate := c.BlockRate
	if blockRate == 0 {
		blockRate = 120
	}

	args := fmt.Sprintf("start --log-level=error --tracer=firehose --store-dir=/data --genesis-block-burst=%d --block-rate=%d --block-size=1500 --genesis-height=0 --server-addr=:9777 --with-reorgs=false --with-skipped-blocks=false", c.Burst, blockRate)
	if c.ExtraReaderArgs != "" {
		args += " " + c.ExtraReaderArgs
	}
	return args
}

// StartDummyBlockchain runs a reader-node/merger/relayer container producing the dummy chain.
func StartDummyBlockchain(ctx context.Context, config ChainConfig) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image: config.image(),
		Cmd: []string{
			"start",
			"reader-node",
			"merger",
			"relayer",
			"-c",
			"",
			"--common-system-shutdown-signal-delay=10s",
			"--advertise-chain-name=acme-dummy-blockchain",
			"--reader-node-path=dummy-blockchain",
			"--reader-node-arguments=" + config.readerArgs(),
			"--advertise-block-id-encoding=hex",
		},
		Env: map[string]string{
			"DLOG": ".*=debug",
		},
		ExposedPorts: []string{"10014/tcp"},
		Mounts: testcontainers.Mounts(
			testcontainers.BindMount(config.TmpDir, "/app/firehose-data/storage/"),
		),
		// Deliberately not wait.ForListeningPort: that one execs a probe inside the container,
		// and the exec itself times out while the node is busy writing a large genesis burst.
		// The log line plus a dial from the host says the same thing without the exec.
		WaitingFor: wait.ForLog("serving gRPC").WithStartupTimeout(config.startupTimeout()),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	endpoint, err := RelayerEndpoint(ctx, container)
	if err != nil {
		return nil, err
	}
	if err := waitDialable(ctx, endpoint, config.startupTimeout()); err != nil {
		return nil, fmt.Errorf("relayer never accepted a connection on %s: %w", endpoint, err)
	}

	return container, nil
}

func waitDialable(ctx context.Context, endpoint string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, err := net.DialTimeout("tcp", endpoint, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout after %s: %w", timeout, err)
		case <-ticker.C:
		}
	}
}

// Tier2Config describes the worker tier.
type Tier2Config struct {
	TmpDir string
	// Secret, when set, makes tier2 require "Authorization: Bearer <secret>" on subrequests.
	Secret string
	// ScratchSpace configures the store scratch space backend, empty for the default.
	ScratchSpace string
	// ReadyTimeout bounds the wait for the app to report ready, 30s when left at zero.
	ReadyTimeout time.Duration
}

// StartTier2 boots a tier2 on a free port and waits for it to become ready.
func StartTier2(ctx context.Context, config Tier2Config, logger *zap.Logger) (*app.Tier2App, string, error) {
	port, err := FindFreePort()
	if err != nil {
		return nil, "", fmt.Errorf("find free port: %w", err)
	}
	endpoint := fmt.Sprintf("localhost:%d", port)

	conf := &app.Tier2Config{
		GRPCListenAddr:        endpoint,
		ServiceDiscoveryURL:   nil,
		BlockExecutionTimeout: 5 * time.Second,
		TmpDir:                filepath.Join(config.TmpDir, "tmp"),
		StoresScratchSpace:    config.ScratchSpace,
	}

	modules := &app.Tier2Modules{
		CheckPendingShutDown: func() bool { return false },
	}

	if config.Secret != "" {
		dauthsecret.Register()
		auth, err := dauth.New(fmt.Sprintf("secret://%s", config.Secret), logger)
		if err != nil {
			return nil, "", fmt.Errorf("secret authenticator: %w", err)
		}
		modules.Authenticator = auth
	} else {
		dauthtrust.Register()
	}

	t2app := app.NewTier2(logger, conf, modules)
	go func() {
		if err := t2app.Run(); err != nil {
			logger.Error("tier2 terminated", zap.Error(err))
		}
	}()

	if err := waitReady(ctx, orDefault(config.ReadyTimeout, 30*time.Second), t2app.IsReady); err != nil {
		return nil, "", fmt.Errorf("tier2 never became ready: %w", err)
	}

	return t2app, endpoint, nil
}

// Tier1Config describes the request tier.
type Tier1Config struct {
	TmpDir string
	// RelayerEndpoint is the block stream tier1 reads live blocks from, normally the mapped
	// 10014 port of the dummy blockchain container.
	RelayerEndpoint string
	// Tier2Endpoint is where subrequests are sent.
	Tier2Endpoint string
	// Tier2Secret must match the tier2 secret when one is configured.
	Tier2Secret string
	// StateBundleSize is the segment size. Small values mean many small jobs, which is what
	// makes a backprocess visible: 100 over a 200k block burst is 2000 segments per stage.
	StateBundleSize uint64
	// MaxSubrequests is how many tier2 jobs may run at once.
	MaxSubrequests uint64
	// MaxWorkersPerSession caps the workers a single request may hold, defaulting to
	// MaxSubrequests. Lower it to observe a request that is throttled by the session pool
	// rather than by the job scheduler.
	MaxWorkersPerSession uint64
	// MetricsPrefix distinguishes the metric set when several tier1s run in one process.
	MetricsPrefix                 string
	LiveBackFillerFinalBlockDelay uint64
	// ReadyTimeout bounds the wait for the app to report ready, two minutes when left at zero.
	// Tier1 bootstraps its block hub from the merged blocks on disk before it answers, so a
	// large genesis burst pushes this well past the handful of seconds a small one needs.
	ReadyTimeout time.Duration
}

func orDefault(value, fallback time.Duration) time.Duration {
	if value != 0 {
		return value
	}
	return fallback
}

// StartTier1 boots a tier1 on a free port and waits for it to become ready.
func StartTier1(ctx context.Context, config Tier1Config, logger *zap.Logger) (*app.Tier1App, string, error) {
	port, err := FindFreePort()
	if err != nil {
		return nil, "", fmt.Errorf("find free port: %w", err)
	}
	endpoint := fmt.Sprintf("localhost:%d", port)

	stateBundleSize := config.StateBundleSize
	if stateBundleSize == 0 {
		stateBundleSize = 100
	}
	maxSubrequests := config.MaxSubrequests
	if maxSubrequests == 0 {
		maxSubrequests = 10
	}
	prefix := config.MetricsPrefix
	if prefix == "" {
		prefix = "test-firehose"
	}
	workersPerSession := config.MaxWorkersPerSession
	if workersPerSession == 0 {
		workersPerSession = maxSubrequests
	}

	// Without this, workers are only ramped up progressively and a short-lived stack spends
	// most of its life below its configured parallelism.
	os.Setenv("SUBSTREAMS_WORKERS_RAMPUP_TIME", "0")

	conf := &app.Tier1Config{
		GRPCListenAddr:                endpoint,
		OneBlocksStoreURL:             filepath.Join(config.TmpDir, "one-blocks"),
		MergedBlocksStoreURL:          filepath.Join(config.TmpDir, "merged-blocks"),
		BlockStreamAddr:               config.RelayerEndpoint,
		MeteringConfig:                "logger://",
		FoundationalStoresConfigPath:  "",
		ForkedBlocksStoreURL:          "",
		GRPCShutdownGracePeriod:       0,
		ServiceDiscoveryURL:           nil,
		BlockExecutionTimeout:         5 * time.Second,
		TmpDir:                        filepath.Join(config.TmpDir, "tmp"),
		StateStoreURL:                 filepath.Join(config.TmpDir, "states"),
		QuickSaveStoreURL:             "",
		StateStoreDefaultTag:          "",
		BlockType:                     BlockType,
		StateBundleSize:               stateBundleSize,
		SubrequestsEndpoint:           config.Tier2Endpoint,
		SubrequestsPlaintext:          true,
		SubrequestsSecret:             config.Tier2Secret,
		MaxSubrequests:                maxSubrequests,
		LiveBackFillerFinalBlockDelay: config.LiveBackFillerFinalBlockDelay,
	}

	// Tier1 is configured with "logger://" metering below, and tier2 inherits the choice through
	// the subrequest. Without the plugin registered every segment fails on the worker side with
	// "no Metering plugin named \"logger\" is currently registered". Done lazily rather than in
	// an init so that merely linking this package into the CLI does not register anything.
	registerMeteringLogger()

	dauthnull.Register()
	auth, err := dauth.New("null://", logger)
	if err != nil {
		return nil, "", fmt.Errorf("null authenticator: %w", err)
	}

	metricset := dmetrics.NewSet()
	sessionPool, err := dsession.New(fmt.Sprintf("local://localhost?max_workers=%d&max_workers_per_session=%d", maxSubrequests, workersPerSession), logger)
	if err != nil {
		return nil, "", fmt.Errorf("session pool: %w", err)
	}

	t1app := app.NewTier1(logger, conf, &app.Tier1Modules{
		HeadTimeDriftMetric:   metricset.NewHeadTimeDrift(prefix),
		HeadBlockNumberMetric: metricset.NewHeadBlockNumber(prefix),
		Authenticator:         auth,
		SessionPool:           sessionPool,
	})
	go func() {
		if err := t1app.Run(); err != nil {
			logger.Error("tier1 terminated", zap.Error(err))
		}
	}()

	if err := waitReady(ctx, orDefault(config.ReadyTimeout, 2*time.Minute), t1app.IsReady); err != nil {
		return nil, "", fmt.Errorf("tier1 never became ready: %w", err)
	}

	return t1app, endpoint, nil
}

var meteringLoggerOnce sync.Once

func registerMeteringLogger() {
	meteringLoggerOnce.Do(meteringlogger.Register)
}

// WaitMergedBlocks blocks until the merger has bundled blocks covering upTo.
//
// Tier1 only reports ready once its block hub can link incoming live blocks, and the hub
// bootstraps from merged blocks. The reader writes a genesis burst far faster than the merger
// bundles it — tens of thousands of one-block files against a merger doing roughly a hundred
// blocks a second — so starting tier1 straight after the container gives it a hub that cannot
// link anything, and it times out reporting "block not linkable after one-block lookup".
func WaitMergedBlocks(ctx context.Context, dataDir string, upTo uint64, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		highest := highestMergedBlock(filepath.Join(dataDir, "merged-blocks"))

		// Each file is named after the first block of the bundle it holds, so the one named
		// upTo-mergedBundleSize is the first that covers upTo.
		if highest+mergedBundleSize >= upTo {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout after %s, merged up to block %d of %d", timeout, highest, upTo)
		case <-ticker.C:
		}
	}
}

// mergedBundleSize is how many blocks the merger puts in one file.
const mergedBundleSize = 100

func highestMergedBlock(dir string) uint64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	var highest uint64
	for _, entry := range entries {
		name, _, found := strings.Cut(entry.Name(), ".")
		if !found {
			continue
		}

		block, err := strconv.ParseUint(name, 10, 64)
		if err == nil && block > highest {
			highest = block
		}
	}

	return highest
}

// RelayerEndpoint resolves the host-side address of the container's relayer.
func RelayerEndpoint(ctx context.Context, container testcontainers.Container) (string, error) {
	port, err := container.MappedPort(ctx, "10014/tcp")
	if err != nil {
		return "", fmt.Errorf("mapped relayer port: %w", err)
	}
	return fmt.Sprintf("localhost:%d", port.Num()), nil
}

// FindFreePort asks the OS for an available TCP port.
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitReady(ctx context.Context, timeout time.Duration, isReady func(context.Context) bool) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout after %s", timeout)
		case <-ticker.C:
			if isReady(ctx) {
				return nil
			}
		}
	}
}
