package tests_e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/streamingfast/substreams/app"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	"github.com/streamingfast/substreams/service/active_requests"
	"github.com/streamingfast/substreams/tools/devenv"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// evictionQuotaCores is the CPU quota the fake cgroup advertises. Eight
// concurrent dev-mode requests burn about 7.5 cores against it, so the evictor
// sees a genuine overload built from real wasm execution — over the threshold
// by enough to fire, but not so far that the batch has to take every request.
const evictionQuotaCores = 6.0

// fakeCgroup writes cpu.max once and then rewrites cpu.stat from the process's
// own CPU time, so the evictor's cgroup reader sees this test's real CPU use
// against a quota we choose. macOS has no cgroups; SUBSTREAMS_CGROUP_DIR is
// what lets the reader run here at all.
func fakeCgroup(t *testing.T, quotaCores float64) string {
	t.Helper()
	dir := t.TempDir()

	period := 100000
	quota := int(quotaCores * float64(period))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpu.max"), []byte(fmt.Sprintf("%d %d\n", quota, period)), 0644))

	writeStat := func() {
		var ru syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
			return
		}
		usec := uint64(ru.Utime.Sec)*1_000_000 + uint64(ru.Utime.Usec) +
			uint64(ru.Stime.Sec)*1_000_000 + uint64(ru.Stime.Usec)
		os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte(fmt.Sprintf("usage_usec %d\n", usec)), 0644)
	}
	writeStat()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				writeStat()
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })

	return dir
}

type streamOutcome struct {
	name      string
	blocks    int
	lastBlock uint64
	err       error
}

func (o streamOutcome) evicted() bool {
	st, ok := status.FromError(o.err)
	return ok && st.Code() == codes.Unavailable
}

// runUntilError streams until the server ends the request or ctx is cancelled,
// reporting how far it got and why it stopped.
func runUntilError(ctx context.Context, name, endpoint string, req *pbsubstreamsrpcv3.Request) streamOutcome {
	out := streamOutcome{name: name}

	conn, closeFunc, callOpts, _, err := client.NewSubstreamsClientConn(client.NewSubstreamsClientConfig(
		client.SubstreamsClientConfigOptions{Endpoint: endpoint, AuthType: client.None, PlainText: true, Agent: "cpu-eviction-test"},
	))
	if err != nil {
		out.err = err
		return out
	}
	defer closeFunc()

	blockStream, err := pbsubstreamsrpcv4.NewStreamClient(conn).Blocks(ctx, req, callOpts...)
	if err != nil {
		out.err = err
		return out
	}
	for {
		resp, err := blockStream.Recv()
		if err != nil {
			out.err = err
			return out
		}
		if datas := resp.GetBlockScopedDatas(); datas != nil {
			out.blocks += len(datas.Items)
			if n := len(datas.Items); n > 0 {
				out.lastBlock = uint64(datas.Items[n-1].Clock.Number)
			}
		}
	}
}

// startTier1WithRetry works around a start-up race in the dummy blockchain: a
// tier1 that connects while the container is still producing the genesis burst
// can see blocks whose parents are not merged yet and give up with "5
// consecutive unlinkable blocks". Waiting and reconnecting is enough — by then
// the burst is on disk. The burst here is much larger than the other e2e tests
// use, which is why only this one needs it.
func startTier1WithRetry(t *testing.T, ctx context.Context, config devenv.Tier1Config, zlog *zap.Logger) (*app.Tier1App, string) {
	t.Helper()
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(5 * time.Second)
		}
		t1app, endpoint, err := devenv.StartTier1(ctx, config, zlog)
		if err == nil {
			return t1app, endpoint
		}
		lastErr = err
		t.Logf("tier1 start attempt %d failed, retrying: %v", attempt+1, err)
	}
	require.NoError(t, lastErr, "tier1 never became ready")
	return nil, ""
}

// TestCPUEviction_OverloadedPodShedsRequests drives the whole path with real
// requests: the fake cgroup reports the process's own CPU against a 3-core
// quota, a wave of dev-mode requests over a long block range pushes it past
// the 90% threshold, and the evictor must go unready, drain, cancel a batch
// with Unavailable, and let the survivors keep streaming.
func TestCPUEviction_OverloadedPodShedsRequests(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 6000)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	t.Setenv("SUBSTREAMS_CGROUP_DIR", fakeCgroup(t, evictionQuotaCores))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app1, endpoint := startTier1WithRetry(t, ctx, devenv.Tier1Config{
		TmpDir:               tmpDir,
		RelayerEndpoint:      relayerEndpoint(t, ctx, container),
		Tier2Endpoint:        t2Endpoint,
		MaxWorkersPerSession: 5,
		MetricsPrefix:        "test-cpu-eviction",
		CPUEviction: active_requests.EvictorConfig{
			Mode:             active_requests.EvictionFull,
			Threshold:        0.90,
			RecoverThreshold: 0.50,
			TargetRatio:      0.60,
			Sustain:          3 * time.Second,
			RecoverSustain:   3 * time.Second,
			Interval:         time.Second,
			Cooldown:         5 * time.Second,
			DrainDelay:       time.Second,
			MinAge:           2 * time.Second,
			MinBurnCores:     0.05,
			NominalCapacity:  8,
		},
	}, zlog)
	defer func() {
		app1.Shutdown(nil)
		app2.Shutdown(nil)
		<-app1.Terminated()
		<-app2.Terminated()
	}()

	require.True(t, app1.IsReady(ctx), "tier1 should start ready")

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.3.0.spkg").Read()
	require.NoError(t, err)

	// Dev mode runs every module of the chain on tier1 itself, so each request
	// is a sustained wasm load on the pod under test rather than on tier2.
	const devCount, liveCount = 8, 2
	devCtx, cancelDev := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelDev()
	liveCtx, cancelLive := context.WithCancel(ctx)
	defer cancelLive()

	// Live production requests at the chain head cost almost no CPU. They are
	// the streams eviction exists to protect, and MinBurnCores must keep them
	// out of the candidate set however overloaded the pod gets.
	live := make([]streamOutcome, liveCount)
	var liveWG sync.WaitGroup
	for i := range liveCount {
		liveWG.Add(1)
		go func() {
			defer liveWG.Done()
			live[i] = runUntilError(liveCtx, fmt.Sprintf("live-%d", i), endpoint, &pbsubstreamsrpcv3.Request{
				StartBlockNum:  -1,
				StopBlockNum:   0,
				ProductionMode: true,
				OutputModule:   "map_events_0",
				Package:        pkg.Package,
			})
		}()
	}

	dev := make([]streamOutcome, devCount)
	var devWG sync.WaitGroup
	for i := range devCount {
		devWG.Add(1)
		go func() {
			defer devWG.Done()
			dev[i] = runUntilError(devCtx, fmt.Sprintf("dev-%d", i), endpoint, &pbsubstreamsrpcv3.Request{
				StartBlockNum:  100,
				StopBlockNum:   6000,
				ProductionMode: false,
				OutputModule:   "map_downstream",
				Package:        pkg.Package,
			})
		}()
	}

	// Watch readiness while the wave runs: the pod must advertise unready
	// before it cancels anything.
	wentUnready := make(chan struct{})
	go func() {
		for {
			select {
			case <-devCtx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				if !app1.IsReady(ctx) {
					close(wentUnready)
					return
				}
			}
		}
	}()

	devWG.Wait()
	cancelLive()
	liveWG.Wait()

	var evicted int
	for _, o := range dev {
		if o.evicted() {
			evicted++
			t.Logf("%s: EVICTED at block %d after %d blocks — %v", o.name, o.lastBlock, o.blocks, o.err)
			continue
		}
		t.Logf("%s: ended at block %d after %d blocks — %v", o.name, o.lastBlock, o.blocks, o.err)
	}

	select {
	case <-wentUnready:
	default:
		t.Error("pod never advertised unready")
	}

	require.Greater(t, evicted, 1, "expected a batch of dev requests cancelled with Unavailable, not one at a time")
	t.Logf("evicted %d of %d dev requests", evicted, devCount)

	// The live streams cost too little CPU to be worth cancelling, so the
	// evictor must have left them alone and they must have kept streaming.
	for _, o := range live {
		require.False(t, o.evicted(), "%s was evicted: %v", o.name, o.err)
		require.Greater(t, o.blocks, 0, "%s received no blocks", o.name)
		t.Logf("%s: survived, %d blocks, last %d", o.name, o.blocks, o.lastBlock)
	}

	// Once the wave is gone CPU falls back under the recover threshold and the
	// pod must take traffic again.
	require.Eventually(t, func() bool { return app1.IsReady(ctx) }, 30*time.Second, time.Second,
		"tier1 should become ready again after the load stops")
}
