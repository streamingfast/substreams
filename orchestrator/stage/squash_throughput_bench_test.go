package stage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/plan"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

// throttledStore simulates object storage upload bandwidth: every byte a WriteObject
// caller reads off its reader sleeps as if the write ran at bytesPerSecond, so a save of a
// large full store costs roughly what it would uploading to real object storage.
type throttledStore struct {
	dstore.Store
	bytesPerSecond float64
}

func (t *throttledStore) WriteObject(ctx context.Context, base string, f io.Reader, metadataKeyValues ...string) error {
	return t.Store.WriteObject(ctx, base, &throttledReader{ctx: ctx, r: f, bytesPerSecond: t.bytesPerSecond}, metadataKeyValues...)
}

func (t *throttledStore) SubStore(subFolder string) (dstore.Store, error) {
	sub, err := t.Store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}
	return &throttledStore{Store: sub, bytesPerSecond: t.bytesPerSecond}, nil
}

type throttledReader struct {
	ctx            context.Context
	r              io.Reader
	bytesPerSecond float64
}

func (t *throttledReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		select {
		case <-time.After(time.Duration(float64(n) / t.bytesPerSecond * float64(time.Second))):
		case <-t.ctx.Done():
			return n, t.ctx.Err()
		}
	}
	return n, err
}

// squashThroughputBaseline reproduces the pre-pipelining singleSquash critical path
// directly against the real store API: merge, then save-and-upload inline, one segment
// fully after another. It is the "before" half of the pipelining throughput comparison.
func squashThroughputBaseline(t *testing.T, objStore dstore.Store, segments, chunkBytes int) time.Duration {
	t.Helper()
	ctx := t.Context()
	cfg, err := store.NewConfig("bench", 0, "bench-hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", objStore, nil, 0, "", "mmap")
	require.NoError(t, err)

	full := cfg.NewFullKV(zap.NewNop())
	defer full.Close()

	start := time.Now()
	for seg := range segments {
		partial := cfg.NewPartialKV(uint64(seg)*10, zap.NewNop())
		partial.Set(0, fmt.Sprintf("seg-%04d", seg), strings.Repeat("x", chunkBytes))
		require.NoError(t, partial.Flush())

		require.NoError(t, full.Merge(partial))
		_, writer, err := full.Save(uint64(seg+1) * 10)
		require.NoError(t, err)
		require.NoError(t, writer.Write(ctx))
	}
	return time.Since(start)
}

// squashThroughputPipelined drives the real Stages.CmdTryMerge path (the pipelined
// singleSquash this PR ships) over the same segments and content. It is the "after" half
// of the comparison.
func squashThroughputPipelined(t *testing.T, objStore dstore.Store, segments, chunkBytes int) time.Duration {
	t.Helper()
	declareTier1Metrics.Do(func() { metrics.DeclareTier1Metrics(zap.NewNop()) })
	ctx := reqctx.WithReqStats(t.Context(), metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))

	cfg, err := store.NewConfig("bench", 0, "bench-hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", objStore, nil, 0, "", "mmap")
	require.NoError(t, err)
	cfgs := map[string]*store.Config{"bench": cfg}

	endBlock := uint64(segments) * 10
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 0, 0, 0, endBlock, endBlock, true, false)
	require.NoError(t, err)
	s := NewStages(ctx, exec.TestGraphStores(storeModule("bench", 0)), reqPlan, nil, cfgs)
	defer s.Close()

	stage := s.stages[0]
	lastSegment := stage.segmenter.LastIndex()
	s.allocSegments(lastSegment)

	modSegmenter := stage.storeModuleStates[0].segmenter
	for segment := modSegmenter.FirstIndex(); segment <= lastSegment; segment++ {
		rng := modSegmenter.Range(segment)
		partial := cfg.NewPartialKV(rng.StartBlock, zap.NewNop())
		partial.Set(0, fmt.Sprintf("seg-%04d", segment), strings.Repeat("x", chunkBytes))
		require.NoError(t, partial.Flush())
		_, writer, err := partial.Save(rng.ExclusiveEndBlock)
		require.NoError(t, err)
		require.NoError(t, writer.Write(ctx))
		s.forceTransition(segment, 0, UnitPartialPresent)
	}

	start := time.Now()
	msg := s.CmdTryMerge(0)()
	finished, ok := msg.(MsgMergeFinished)
	require.True(t, ok, "squash run failed: %+v", msg)
	elapsed := time.Since(start)

	require.Len(t, finished.Merged, lastSegment-modSegmenter.FirstIndex()+1)
	return elapsed
}

// TestSquashPipeliningThroughput is the measured before/after this PR's pipelining is
// expected to produce: the baseline pays every segment's full-store upload inline before
// the next segment can merge; the pipelined path overlaps them, bounded by
// squasherUploadConcurrency. Reported as a log line rather than a tight ratio assertion,
// since the exact speedup depends on the machine's scheduler latency relative to the
// simulated upload bandwidth below — only that pipelining must not be slower holds up
// across machines.
func TestSquashPipeliningThroughput(t *testing.T) {
	const (
		segments       = 12
		chunkBytes     = 1 << 20 // 1 MiB of new data per segment -> ~12 MiB full store by the end
		bytesPerSecond = 24 << 20 // simulated upload bandwidth: 24 MiB/s
	)

	before := func() time.Duration {
		base, err := dstore.NewStore("file://"+t.TempDir(), "zst", "zstd", true)
		require.NoError(t, err)
		return squashThroughputBaseline(t, &throttledStore{Store: base, bytesPerSecond: bytesPerSecond}, segments, chunkBytes)
	}()

	after := func() time.Duration {
		base, err := dstore.NewStore("file://"+t.TempDir(), "zst", "zstd", true)
		require.NoError(t, err)
		return squashThroughputPipelined(t, &throttledStore{Store: base, bytesPerSecond: bytesPerSecond}, segments, chunkBytes)
	}()

	t.Logf("squash throughput over %d segments (~%s final store, simulated %s/s upload): before=%s after=%s (%.1fx)",
		segments, humanizeBytes(segments*chunkBytes), humanizeBytes(bytesPerSecond), before, after, before.Seconds()/after.Seconds())

	require.Less(t, after, before, "pipelining must not make the same squash run slower")
}

func humanizeBytes(n int) string {
	const mib = 1 << 20
	return fmt.Sprintf("%.1fMiB", float64(n)/mib)
}
