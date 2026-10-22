package stage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/plan"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

// failOnceStore fails the first WriteObject attempt for each distinct filename (as a
// transient upload error would), then succeeds on every retry of that same filename.
type failOnceStore struct {
	dstore.Store
	mu     sync.Mutex
	failed map[string]bool
}

func (f *failOnceStore) WriteObject(ctx context.Context, base string, r io.Reader, metadataKeyValues ...string) error {
	if strings.HasSuffix(base, ".kv") {
		f.mu.Lock()
		alreadyFailed := f.failed[base]
		f.failed[base] = true
		f.mu.Unlock()
		if !alreadyFailed {
			return errors.New("injected transient upload failure")
		}
	}
	return f.Store.WriteObject(ctx, base, r, metadataKeyValues...)
}

func (f *failOnceStore) SubStore(subFolder string) (dstore.Store, error) {
	sub, err := f.Store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}
	return &failOnceStore{Store: sub, failed: f.failed}, nil
}

// TestSquashRetryUploadsExactSnapshot guards against a pipelined upload retry saving a
// later segment's content under an earlier segment's filename: segment 1's first upload
// attempt fails, and while its retry backs off, segments 2-4 merge and upload
// concurrently (that's the point of pipelining) before segment 1's retry succeeds.
// Segment 1's saved file must still hold exactly segment 1's content.
func TestSquashRetryUploadsExactSnapshot(t *testing.T) {
	declareTier1Metrics.Do(func() { metrics.DeclareTier1Metrics(zap.NewNop()) })
	ctx := reqctx.WithReqStats(t.Context(), metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))

	base, err := dstore.NewStore("file://"+t.TempDir(), "zst", "zstd", true)
	require.NoError(t, err)
	failing := &failOnceStore{Store: base, failed: make(map[string]bool)}

	const segments = 4
	cfg, err := store.NewConfig("bench", 0, "bench-hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", failing, nil, 0, "", "mmap")
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
		partial.Set(0, fmt.Sprintf("seg-%04d", segment), "x")
		require.NoError(t, partial.Flush())
		_, writer, err := partial.Save(rng.ExclusiveEndBlock)
		require.NoError(t, err)
		require.NoError(t, writer.Write(ctx))
		s.forceTransition(segment, 0, UnitPartialPresent)
	}

	msg := s.CmdTryMerge(0)()
	finished, ok := msg.(MsgMergeFinished)
	require.True(t, ok, "squash run failed: %+v", msg)
	require.Len(t, finished.Merged, lastSegment-modSegmenter.FirstIndex()+1)

	// Reload segment 1's saved file directly and check it holds exactly its own
	// segment's keys, not any later segment's.
	firstRng := modSegmenter.Range(modSegmenter.FirstIndex())
	fullKVFile := store.NewCompleteFileInfo("bench", cfg.ModuleInitialBlock(), firstRng.ExclusiveEndBlock)
	reload := cfg.NewFullKV(zap.NewNop())
	defer reload.Close()
	require.NoError(t, reload.Load(ctx, fullKVFile))

	for segment := modSegmenter.FirstIndex(); segment <= lastSegment; segment++ {
		key := fmt.Sprintf("seg-%04d", segment)
		_, found := reload.GetLast(key)
		if segment == modSegmenter.FirstIndex() {
			assert.True(t, found, "segment 1's saved file is missing its own key %q", key)
		} else {
			assert.False(t, found, "segment 1's saved file holds segment %d's key %q — retry uploaded a later snapshot under segment 1's filename", segment, key)
		}
	}
}

// TestSquashStepSurfacesUploadErrorNotBareCanceled guards against squashStep reporting a
// pipelined upload failure as a bare context.Canceled: once uploads.ctx is done, squashStep
// used to return uploads.ctx.Err() directly, discarding the real error (e.g. "uploading full
// store at segment 4: disk full") that errgroup recorded. Downstream, service/error.go's
// toConnectError and tier1's reqStats.SetError both special-case context.Canceled as a benign
// client cancellation, so that bare error would misreport a genuine storage failure as the
// caller having canceled the request. squashStep must instead surface the real error via
// uploads.Wait().
func TestSquashStepSurfacesUploadErrorNotBareCanceled(t *testing.T) {
	uploads := newSquashUploads(t.Context())
	sentinel := errors.New("uploading full store at segment 4: disk full")
	uploads.Go(func() error { return sentinel })
	require.ErrorIs(t, uploads.Wait(), sentinel) // drain so uploads.ctx is observably Done below

	q := &localStepSquasher{stages: &Stages{ctx: t.Context()}, uploads: uploads}
	err := q.squashStep(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "squashStep must surface the real upload error")
	assert.NotErrorIs(t, err, context.Canceled, "squashStep must not report a pipelined upload failure as a bare context.Canceled")
}
