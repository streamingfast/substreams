package stage

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// copyCountingStore counts the objects copied through it and through its sub stores.
type copyCountingStore struct {
	dstore.Store
	copies *atomic.Int64
}

func (c *copyCountingStore) CopyObject(ctx context.Context, src, dest string) error {
	c.copies.Add(1)
	return c.Store.CopyObject(ctx, src, dest)
}

func (c *copyCountingStore) SubStore(subFolder string) (dstore.Store, error) {
	sub, err := c.Store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}
	return &copyCountingStore{Store: sub, copies: c.copies}, nil
}

// partialContent is what the partial of one segment holds. The zero value is an empty
// partial.
type partialContent struct {
	set          map[string]string
	deletePrefix string
}

type squashScenario struct {
	// endBlock is the exclusive end of the store segments, which are 10 blocks long.
	endBlock uint64
	// initialKeys is the full store at block 10, with segment 0 already merged. When nil,
	// the run starts at the module's first segment, with no full store yet.
	initialKeys map[string]string
	// partials holds one entry per segment of the run.
	partials []partialContent
}

type squashOutcome struct {
	cfg    *store.Config
	copies int64
	// fullKVs is the decompressed content of every full store file, by file name.
	fullKVs map[string][]byte
}

// runSquashScenario writes the scenario's files to zstd-compressed local storage, the way
// tier1 does, and squashes all its partials with one squash run.
// declareTier1Metrics declares the tier1 metrics the squash increments, which tier1
// declares at startup.
var declareTier1Metrics sync.Once

func runSquashScenario(t *testing.T, sc squashScenario, copyEmpty bool) squashOutcome {
	t.Helper()
	declareTier1Metrics.Do(func() { metrics.DeclareTier1Metrics(zap.NewNop()) })
	ctx := reqctx.WithReqStats(context.Background(), metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))

	previous := copyOverEmptyPartials
	copyOverEmptyPartials = copyEmpty
	t.Cleanup(func() { copyOverEmptyPartials = previous })

	base, err := dstore.NewStore("file://"+t.TempDir(), "zst", "zstd", true)
	require.NoError(t, err)
	counting := &copyCountingStore{Store: base, copies: &atomic.Int64{}}
	cfg := testStoreConfig(t, "", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", counting)

	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 0, 0, 0, sc.endBlock, sc.endBlock, true)
	require.NoError(t, err)
	s := NewStages(ctx, exec.TestGraphStagedModules(0, 0, 0, 0, 0), reqPlan, nil, store.ConfigMap{"": cfg})
	t.Cleanup(s.Close)

	firstSegment := 0
	if sc.initialKeys != nil {
		full := cfg.NewFullKV(zap.NewNop())
		for k, v := range sc.initialKeys {
			full.Set(0, k, v)
		}
		require.NoError(t, full.Flush())
		_, writer, err := full.Save(10)
		require.NoError(t, err)
		require.NoError(t, writer.Write(ctx))
		full.Close()
		firstSegment = 1
	}
	lastSegment := firstSegment + len(sc.partials) - 1
	require.Equal(t, s.stages[0].segmenter.LastIndex(), lastSegment, "the scenario must cover every store segment")

	s.allocSegments(lastSegment)
	if firstSegment == 1 {
		s.forceTransition(0, 0, UnitMerging)
		s.MergeCompleted(unit(0, 0))
	}
	for i, content := range sc.partials {
		segment := firstSegment + i
		rng := s.stages[0].segmenter.Range(segment)
		partial := cfg.NewPartialKV(rng.StartBlock, zap.NewNop())
		for k, v := range content.set {
			partial.Set(0, k, v)
		}
		if content.deletePrefix != "" {
			partial.DeletePrefix(0, content.deletePrefix)
		}
		require.NoError(t, partial.Flush())
		_, writer, err := partial.Save(rng.ExclusiveEndBlock)
		require.NoError(t, err)
		require.NoError(t, writer.Write(ctx))
		s.forceTransition(segment, 0, UnitPartialPresent)
	}

	msg := s.CmdTryMerge(0)()
	finished, ok := msg.(MsgMergeFinished)
	require.True(t, ok, "squash run failed: %+v", msg)
	var want []Unit
	for segment := firstSegment; segment <= lastSegment; segment++ {
		want = append(want, unit(segment, 0))
	}
	assert.Equal(t, want, finished.Merged)
	assert.Empty(t, finished.Unmerged)

	// A partial is deleted once a full store covers its segment, which a segment shorter
	// than the interval never gets.
	var wantPartials []string
	for segment := firstSegment; segment <= lastSegment; segment++ {
		if !s.stages[0].segmenter.EndsOnInterval(segment) {
			wantPartials = append(wantPartials, store.PartialFileName(s.stages[0].segmenter.Range(segment)))
		}
	}
	states, err := base.SubStore("hash/states")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return slices.Equal(wantPartials, listStateFiles(t, states, ".partial"))
	}, 5*time.Second, 10*time.Millisecond, "only partials not covered by a full store remain: want %v, have %v", wantPartials, listStateFiles(t, states, ".partial"))

	out := squashOutcome{cfg: cfg, copies: counting.copies.Load(), fullKVs: map[string][]byte{}}
	for _, name := range listStateFiles(t, states, ".kv") {
		reader, err := states.OpenObject(ctx, name)
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		reader.Close()
		require.NoError(t, err)
		out.fullKVs[name] = content
	}
	return out
}

func listStateFiles(t *testing.T, states dstore.Store, kind string) []string {
	t.Helper()
	var out []string
	require.NoError(t, states.Walk(context.Background(), "", func(filename string) error {
		if strings.Contains(filename, kind) {
			out = append(out, filename)
		}
		return nil
	}))
	return out
}

func loadFullKVKeys(t *testing.T, cfg *store.Config, endBlock uint64) map[string]string {
	t.Helper()
	full := cfg.NewFullKV(zap.NewNop())
	defer full.Close()
	require.NoError(t, full.Load(context.Background(), store.NewCompleteFileInfo("", 0, endBlock)))
	out := map[string]string{}
	require.NoError(t, full.Iter(func(key string, value []byte) error {
		out[key] = string(value)
		return nil
	}))
	return out
}

func TestSquashCopiesOverEmptyPartials(t *testing.T) {
	empty := partialContent{}

	tests := []struct {
		name       string
		scenario   squashScenario
		wantCopies int64
		// wantKeys is the content expected in some of the full stores, by end block.
		wantKeys  map[uint64]map[string]string
		wantFiles int
	}{
		{
			name: "empty partials around data",
			scenario: squashScenario{
				endBlock:    100,
				initialKeys: map[string]string{"a": "1"},
				partials: []partialContent{
					empty, empty, // segment 1 is squashed normally, the store is not in memory yet
					{set: map[string]string{"b": "2"}},
					empty,
					{deletePrefix: "a"}, // only a deleted prefix: not empty
					empty, empty,
					{set: map[string]string{"c": "3"}},
					empty,
				},
			},
			wantCopies: 5, // segments 2, 4, 6, 7 and 9
			wantKeys: map[uint64]map[string]string{
				30:  {"a": "1"},
				40:  {"a": "1", "b": "2"},
				50:  {"a": "1", "b": "2"},
				70:  {"b": "2"},
				100: {"b": "2", "c": "3"},
			},
			wantFiles: 10,
		},
		{
			name: "last segment shorter than the interval is never saved",
			scenario: squashScenario{
				endBlock:    95,
				initialKeys: map[string]string{"a": "1"},
				partials: []partialContent{
					{set: map[string]string{"b": "2"}},
					empty, empty, empty, empty, empty, empty, empty, empty,
				},
			},
			wantCopies: 7, // segments 2 to 8, segment 9 ends at 95
			wantKeys: map[uint64]map[string]string{
				90: {"a": "1", "b": "2"},
			},
			wantFiles: 9,
		},
		{
			name: "run starting at the module's first segment",
			scenario: squashScenario{
				endBlock: 50,
				partials: []partialContent{
					empty, // first segment: no earlier full store to copy
					empty,
					{set: map[string]string{"a": "1"}},
					empty, empty,
				},
			},
			wantCopies: 3, // segments 1, 3 and 4
			wantKeys: map[uint64]map[string]string{
				10: {},
				20: {},
				50: {"a": "1"},
			},
			wantFiles: 5,
		},
		{
			name: "no empty partial",
			scenario: squashScenario{
				endBlock:    40,
				initialKeys: map[string]string{"a": "1"},
				partials: []partialContent{
					{set: map[string]string{"b": "2"}},
					{set: map[string]string{"c": "3"}},
					{deletePrefix: "b"},
				},
			},
			wantCopies: 0,
			wantKeys: map[uint64]map[string]string{
				40: {"a": "1", "c": "3"},
			},
			wantFiles: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regular := runSquashScenario(t, tt.scenario, false)
			copied := runSquashScenario(t, tt.scenario, true)

			assert.Zero(t, regular.copies)
			assert.Equal(t, tt.wantCopies, copied.copies)

			assert.Len(t, copied.fullKVs, tt.wantFiles)
			assert.Equal(t, regular.fullKVs, copied.fullKVs, "copying must produce the same full stores as merging")

			for endBlock, want := range tt.wantKeys {
				assert.Equal(t, want, loadFullKVKeys(t, copied.cfg, endBlock), "full store at block %d", endBlock)
			}
		})
	}
}
