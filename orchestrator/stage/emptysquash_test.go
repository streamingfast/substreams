package stage

import (
	"context"
	"errors"
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

// copyCountingStore counts the objects copied through it and through its sub stores, and
// fails every copy after the first failAfter ones when failAfter is not zero.
type copyCountingStore struct {
	dstore.Store
	copies    *atomic.Int64
	failAfter int64
}

func (c *copyCountingStore) CopyObject(ctx context.Context, src, dest string) error {
	if n := c.copies.Add(1); c.failAfter > 0 && n > c.failAfter {
		return errors.New("copy failure injected by the test")
	}
	return c.Store.CopyObject(ctx, src, dest)
}

func (c *copyCountingStore) SubStore(subFolder string) (dstore.Store, error) {
	sub, err := c.Store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}
	return &copyCountingStore{Store: sub, copies: c.copies, failAfter: c.failAfter}, nil
}

// partialContent is what the partial of one segment holds. The zero value is an empty
// partial.
type partialContent struct {
	set          map[string]string
	deletePrefix string
}

// storeScenario is one store module of the squashed stage.
type storeScenario struct {
	name      string
	initBlock uint64
	// initialKeys is the module's full store at block 10, written when the run starts at
	// segment 1 and the module started before it.
	initialKeys map[string]string
	// partials is the content of the module's partial for each segment of the run. The
	// segments missing here get an empty partial.
	partials map[int]partialContent
}

type squashScenario struct {
	// endBlock is the exclusive end of the store segments, which are 10 blocks long.
	endBlock uint64
	// firstSegment is where the run starts, 0 or 1. When 1, segment 0 is already merged.
	firstSegment int
	stores       []storeScenario
	// secondRunFrom, when not zero, splits the squash in two runs: the partials of the
	// segments from it are only reported present once the first run is done.
	secondRunFrom int
	// failCopiesAfter, when not zero, fails every copy after that many.
	failCopiesAfter int64
	// extraPartials allows partials to remain on segments that have a full store, which
	// the regular squash leaves behind when it finds the full store already written.
	extraPartials bool
}

type squashOutcome struct {
	cfgs   map[string]*store.Config
	copies int64
	// fullKVs is the decompressed content of every full store file, by store then file.
	fullKVs map[string]map[string][]byte
	// partials is the partial files left in storage, by store.
	partials map[string][]string
}

// declareTier1Metrics declares the tier1 metrics the squash increments, which tier1
// declares at startup.
var declareTier1Metrics sync.Once

func storeModule(name string, initBlock uint64) *pbsubstreams.Module {
	return &pbsubstreams.Module{
		Name:         name,
		InitialBlock: initBlock,
		Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
	}
}

// runSquashScenario writes the scenario's files to zstd-compressed local storage, the way
// tier1 does, and squashes all of its partials on stage 0 as the scheduler would.
func runSquashScenario(t *testing.T, sc squashScenario, copyEmpty bool) squashOutcome {
	t.Helper()
	declareTier1Metrics.Do(func() { metrics.DeclareTier1Metrics(zap.NewNop()) })
	ctx := reqctx.WithReqStats(context.Background(), metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))

	previous := copyOverEmptyPartials
	copyOverEmptyPartials = copyEmpty
	t.Cleanup(func() { copyOverEmptyPartials = previous })

	base, err := dstore.NewStore("file://"+t.TempDir(), "zst", "zstd", true)
	require.NoError(t, err)
	counting := &copyCountingStore{Store: base, copies: &atomic.Int64{}, failAfter: sc.failCopiesAfter}

	var modules []*pbsubstreams.Module
	cfgs := make(map[string]*store.Config)
	for _, st := range sc.stores {
		modules = append(modules, storeModule(st.name, st.initBlock))
		cfgs[st.name] = testStoreConfig(t, st.name, st.initBlock, "hash-"+st.name, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", counting)
	}

	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 0, 0, 0, sc.endBlock, sc.endBlock, true)
	require.NoError(t, err)
	s := NewStages(ctx, exec.TestGraphStores(modules...), reqPlan, nil, cfgs)
	t.Cleanup(s.Close)

	stage := s.stages[0]
	lastSegment := stage.segmenter.LastIndex()
	s.allocSegments(lastSegment)

	for i, st := range sc.stores {
		cfg := cfgs[st.name]
		modSegmenter := stage.storeModuleStates[i].segmenter
		if sc.firstSegment == 1 && modSegmenter.FirstIndex() == 0 {
			full := cfg.NewFullKV(zap.NewNop())
			for k, v := range st.initialKeys {
				full.Set(0, k, v)
			}
			require.NoError(t, full.Flush())
			_, writer, err := full.Save(10)
			require.NoError(t, err)
			require.NoError(t, writer.Write(ctx))
			full.Close()
		}
		for segment := max(sc.firstSegment, modSegmenter.FirstIndex()); segment <= lastSegment; segment++ {
			rng := modSegmenter.Range(segment)
			content := st.partials[segment]
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
		}
	}
	if sc.firstSegment == 1 {
		s.forceTransition(0, 0, UnitMerging)
		s.MergeCompleted(unit(0, 0))
	}

	runs := [][2]int{{sc.firstSegment, lastSegment}}
	if sc.secondRunFrom != 0 {
		runs = [][2]int{{sc.firstSegment, sc.secondRunFrom - 1}, {sc.secondRunFrom, lastSegment}}
	}
	var merged []Unit
	for _, run := range runs {
		for segment := run[0]; segment <= run[1]; segment++ {
			s.forceTransition(segment, 0, UnitPartialPresent)
		}
		msg := s.CmdTryMerge(0)()
		finished, ok := msg.(MsgMergeFinished)
		require.True(t, ok, "squash run failed: %+v", msg)
		require.Empty(t, finished.Unmerged)
		s.MergeRunCompleted(finished.Stage, finished.Merged, finished.Unmerged)
		merged = append(merged, finished.Merged...)
	}
	var want []Unit
	for segment := sc.firstSegment; segment <= lastSegment; segment++ {
		want = append(want, unit(segment, 0))
	}
	require.Equal(t, want, merged)

	out := squashOutcome{
		cfgs:     cfgs,
		fullKVs:  make(map[string]map[string][]byte),
		partials: make(map[string][]string),
	}
	for i, st := range sc.stores {
		modSegmenter := stage.storeModuleStates[i].segmenter
		states, err := base.SubStore("hash-" + st.name + "/states")
		require.NoError(t, err)

		// A partial is deleted once a full store covers its segment, which a segment
		// shorter than the interval never gets.
		var wantPartials []string
		for segment := max(sc.firstSegment, modSegmenter.FirstIndex()); segment <= lastSegment; segment++ {
			if !modSegmenter.EndsOnInterval(segment) {
				wantPartials = append(wantPartials, store.PartialFileName(modSegmenter.Range(segment)))
			}
		}
		require.Eventually(t, func() bool {
			have := listStateFiles(t, states, ".partial")
			if sc.extraPartials {
				return !slices.ContainsFunc(wantPartials, func(p string) bool { return !slices.Contains(have, p) })
			}
			return slices.Equal(wantPartials, have)
		}, 5*time.Second, 10*time.Millisecond, "store %s: exactly the partials not covered by a full store remain", st.name)
		out.partials[st.name] = listStateFiles(t, states, ".partial")

		out.fullKVs[st.name] = make(map[string][]byte)
		fullEndBlocks := make(map[string]bool)
		for _, name := range listStateFiles(t, states, ".kv") {
			reader, err := states.OpenObject(ctx, name)
			require.NoError(t, err)
			content, err := io.ReadAll(reader)
			reader.Close()
			require.NoError(t, err)
			out.fullKVs[st.name][name] = content
			endBlock, _, _ := strings.Cut(name, "-")
			fullEndBlocks[endBlock] = true
		}
		for _, p := range out.partials[st.name] {
			if slices.Contains(wantPartials, p) {
				continue
			}
			endBlock, _, _ := strings.Cut(p, "-")
			assert.True(t, fullEndBlocks[endBlock], "store %s: partial %s left behind without a full store", st.name, p)
		}
	}
	out.copies = counting.copies.Load()
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
	require.NoError(t, full.Load(context.Background(), store.NewCompleteFileInfo(cfg.Name(), cfg.ModuleInitialBlock(), endBlock)))
	out := map[string]string{}
	require.NoError(t, full.Iter(func(key string, value []byte) error {
		out[key] = string(value)
		return nil
	}))
	return out
}

func TestSquashCopiesOverEmptyPartials(t *testing.T) {
	tests := []struct {
		name     string
		scenario squashScenario
		// wantCopies is the number of copies made, or -1 to not check it.
		wantCopies int64
		// wantKeys is the content expected in some full stores, by store then end block.
		wantKeys map[string]map[uint64]map[string]string
		// wantFiles is the number of full store files expected, by store.
		wantFiles map[string]int
	}{
		{
			name: "empty partials around data",
			scenario: squashScenario{
				endBlock:     100,
				firstSegment: 1,
				stores: []storeScenario{{
					name:        "a",
					initialKeys: map[string]string{"a": "1"},
					partials: map[int]partialContent{
						// 1 and 2 empty: 1 is squashed normally, the store is not in memory yet
						3: {set: map[string]string{"b": "2"}},
						5: {deletePrefix: "a"}, // only a deleted prefix: not empty
						8: {set: map[string]string{"c": "3"}},
					},
				}},
			},
			wantCopies: 5, // segments 2, 4, 6, 7 and 9
			wantKeys: map[string]map[uint64]map[string]string{"a": {
				30:  {"a": "1"},
				40:  {"a": "1", "b": "2"},
				50:  {"a": "1", "b": "2"},
				70:  {"b": "2"},
				100: {"b": "2", "c": "3"},
			}},
			wantFiles: map[string]int{"a": 10},
		},
		{
			name: "last segment shorter than the interval is never saved",
			scenario: squashScenario{
				endBlock:     95,
				firstSegment: 1,
				stores: []storeScenario{{
					name:        "a",
					initialKeys: map[string]string{"a": "1"},
					partials:    map[int]partialContent{1: {set: map[string]string{"b": "2"}}},
				}},
			},
			wantCopies: 7, // segments 2 to 8, segment 9 ends at 95
			wantKeys:   map[string]map[uint64]map[string]string{"a": {90: {"a": "1", "b": "2"}}},
			wantFiles:  map[string]int{"a": 9},
		},
		{
			name: "run starting at the module's first segment",
			scenario: squashScenario{
				endBlock: 50,
				stores: []storeScenario{{
					name: "a",
					// 0 is the first segment: no earlier full store to copy
					partials: map[int]partialContent{2: {set: map[string]string{"a": "1"}}},
				}},
			},
			wantCopies: 3, // segments 1, 3 and 4
			wantKeys:   map[string]map[uint64]map[string]string{"a": {10: {}, 20: {}, 50: {"a": "1"}}},
			wantFiles:  map[string]int{"a": 5},
		},
		{
			name: "no empty partial",
			scenario: squashScenario{
				endBlock:     40,
				firstSegment: 1,
				stores: []storeScenario{{
					name:        "a",
					initialKeys: map[string]string{"a": "1"},
					partials: map[int]partialContent{
						1: {set: map[string]string{"b": "2"}},
						2: {set: map[string]string{"c": "3"}},
						3: {deletePrefix: "b"},
					},
				}},
			},
			wantCopies: 0,
			wantKeys:   map[string]map[uint64]map[string]string{"a": {40: {"a": "1", "c": "3"}}},
			wantFiles:  map[string]int{"a": 4},
		},
		{
			name: "two stores, one starting within the run",
			scenario: squashScenario{
				endBlock:     60,
				firstSegment: 1,
				stores: []storeScenario{
					{name: "a", initialKeys: map[string]string{"a": "1"}},
					{
						name:      "b",
						initBlock: 25, // starts in segment 2, which is therefore never copied
						partials:  map[int]partialContent{2: {set: map[string]string{"b": "1"}}},
					},
				},
			},
			wantCopies: 6, // segments 3, 4 and 5 of both stores
			wantKeys: map[string]map[uint64]map[string]string{
				"a": {30: {"a": "1"}, 60: {"a": "1"}},
				"b": {30: {"b": "1"}, 60: {"b": "1"}},
			},
			wantFiles: map[string]int{"a": 6, "b": 4},
		},
		{
			name: "one store empty while the other holds data",
			scenario: squashScenario{
				endBlock:     60,
				firstSegment: 1,
				stores: []storeScenario{
					{
						name:        "a",
						initialKeys: map[string]string{"a": "1"},
						partials:    map[int]partialContent{3: {set: map[string]string{"a": "2"}}},
					},
					{
						name:        "b",
						initialKeys: map[string]string{"b": "1"},
						partials:    map[int]partialContent{4: {deletePrefix: "b"}},
					},
				},
			},
			wantCopies: 4, // segments 2 and 5 of both stores; 3 and 4 hold data in one of them
			wantKeys: map[string]map[uint64]map[string]string{
				"a": {40: {"a": "2"}, 60: {"a": "2"}},
				"b": {40: {"b": "1"}, 50: {}, 60: {}},
			},
			wantFiles: map[string]int{"a": 6, "b": 6},
		},
		{
			name: "two runs, the second starting on empty partials",
			scenario: squashScenario{
				endBlock:      60,
				firstSegment:  1,
				secondRunFrom: 3,
				stores: []storeScenario{{
					name:        "a",
					initialKeys: map[string]string{"a": "1"},
					partials:    map[int]partialContent{1: {set: map[string]string{"b": "2"}}},
				}},
			},
			wantCopies: 4, // segment 2 in the first run, 3 to 5 in the second
			wantKeys:   map[string]map[uint64]map[string]string{"a": {60: {"a": "1", "b": "2"}}},
			wantFiles:  map[string]int{"a": 6},
		},
		{
			name: "failing copies fall back to the regular squash",
			scenario: squashScenario{
				endBlock:        60,
				firstSegment:    1,
				failCopiesAfter: 1,
				extraPartials:   true, // the copy that succeeded makes the fallback skip that partial
				stores: []storeScenario{{
					name:        "a",
					initialKeys: map[string]string{"a": "1"},
					partials:    map[int]partialContent{1: {set: map[string]string{"b": "2"}}},
				}},
			},
			wantCopies: -1, // retried copies all count
			wantKeys:   map[string]map[uint64]map[string]string{"a": {60: {"a": "1", "b": "2"}}},
			wantFiles:  map[string]int{"a": 6},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regular := runSquashScenario(t, tt.scenario, false)
			copied := runSquashScenario(t, tt.scenario, true)

			assert.Zero(t, regular.copies)
			if tt.wantCopies >= 0 {
				assert.Equal(t, tt.wantCopies, copied.copies)
			} else {
				assert.Positive(t, copied.copies)
			}

			for name, want := range tt.wantFiles {
				assert.Len(t, copied.fullKVs[name], want, "full stores of %s", name)
			}
			assert.Equal(t, regular.fullKVs, copied.fullKVs, "copying must produce the same full stores as merging")
			if !tt.scenario.extraPartials {
				assert.Equal(t, regular.partials, copied.partials, "copying must leave the same partials as merging")
			}

			for name, byEnd := range tt.wantKeys {
				for endBlock, want := range byEnd {
					assert.Equal(t, want, loadFullKVKeys(t, copied.cfgs[name], endBlock), "full store of %s at block %d", name, endBlock)
				}
			}
		})
	}
}
