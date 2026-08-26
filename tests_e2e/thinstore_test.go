package tests_e2e

// Thin store suite: tier1 resuming from the last remaining store snapshot after
// intermediate snapshots, mapper outputs and index files were deleted.
//
// Opt-in, it takes ~15 minutes:
//
//	SUBSTREAMS_E2E_THINSTORE=true go test ./tests_e2e/ -run TestThinstore -v -timeout 2h
//
// Knobs:
//
//	SUBSTREAMS_E2E_THINSTORE_BLOCKS           blocks in the baseline range (default 15000)
//	SUBSTREAMS_E2E_THINSTORE_FUZZ_ITERATIONS  fuzz iterations after the scenarios (default 3, 0 disables)
//	SUBSTREAMS_E2E_THINSTORE_FUZZ_SEED        fuzz seed (default 1), printed on failure for replay
//	SUBSTREAMS_E2E_THINSTORE_FUZZ_CHAOS       also delete random files while fuzz queries run
//	SUBSTREAMS_E2E_THINSTORE_QUERY_TIMEOUT    a query still running after this is reported as hung (default 10m)
//
// Tier1 and tier2 run in the test process; DLOG=".*=info" shows their logs, including the
// `substreams request progress` lines that carry the scheduler's unit states on a stall.
//
// The package under test (thinstore/) only reads the Clock, so it runs on the dummy
// blockchain and every map_out value is a pure function of the block number: any run is
// compared block by block with a baseline computed first. Four stages, several mappers and
// stores per stage plus a block index:
//
//	stage 1: index_parity, map_a, store_count, store_a_max
//	stage 2: store_sum, store_a_last, map_b
//	stage 3: store_c, map_c (filtered by index_parity)
//	stage 4: map_out

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tools/devenv"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

const (
	thinstoreSpkg        = "./thinstore/thinstore-v0.2.0.spkg"
	thinstoreSegment     = 100
	thinstoreKeepEvery   = 1000
	thinstoreChaosPruned = "does not exist (it may have been pruned)"
)

// errThinstoreHung marks a query that never completed: the suite stops there, a hung
// request would only make every following query look hung too.
var errThinstoreHung = errors.New("HUNG: no completion")

var thinstoreFileRegex = regexp.MustCompile(`^(\d{10})-(\d{10})\.(kv|partial|output|index)`)

func thinstoreQueryLimit() time.Duration {
	if v := os.Getenv("SUBSTREAMS_E2E_THINSTORE_QUERY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 10 * time.Minute
}

func thinstoreEnvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

// thinstoreStack is the running tier1/tier2 over a dummy chain plus everything the
// scenarios need to know about the state store on disk.
type thinstoreStack struct {
	t        *testing.T
	ctx      context.Context
	endpoint string
	pkg      *pbsubstreams.Package
	hashes   map[string]string // module name -> module hash
	stateDir string
	last     uint64 // exclusive end of the baseline range
	baseline map[uint64]string
}

func startThinstoreStack(t *testing.T, blocks uint64) *thinstoreStack {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// The merger needs blocks beyond the last bundle to seal it.
	container, err := devenv.StartDummyBlockchain(ctx, devenv.ChainConfig{
		Image:          latestDummyBlockchainImage,
		TmpDir:         tmpDir,
		Burst:          int(blocks) + 500,
		BlockRate:      60,
		StartupTimeout: 10 * time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(func() { container.Terminate(ctx, testcontainers.StopTimeout(0)) })

	// Wait for the merger to have written every bundle of the baseline range before tier1
	// starts: its block hub bootstraps from the merged blocks, and joining the live stream
	// in the middle of the burst leaves it with unlinkable blocks.
	mergedDir := filepath.Join(tmpDir, "merged-blocks")
	require.Eventually(t, func() bool {
		entries, _ := os.ReadDir(mergedDir)
		return len(entries) >= int(blocks/100)
	}, 10*time.Minute, 2*time.Second, "merged blocks up to %d", blocks)

	t2app, t2Endpoint := startTier2(t, ctx, devenv.Tier2Config{TmpDir: tmpDir}, zlog)
	t1app, endpoint := startTier1(t, ctx, devenv.Tier1Config{
		TmpDir:          tmpDir,
		RelayerEndpoint: relayerEndpoint(t, ctx, container),
		Tier2Endpoint:   t2Endpoint,
		StateBundleSize: thinstoreSegment,
		MaxSubrequests:  8,
		MetricsPrefix:   "test-thinstore",
		ReadyTimeout:    10 * time.Minute,
	}, zlog)
	t.Cleanup(func() {
		t1app.Shutdown(nil)
		t2app.Shutdown(nil)
		<-t1app.Terminated()
		<-t2app.Terminated()
	})

	pkg, err := manifest.MustNewReader(thinstoreSpkg).Read()
	require.NoError(t, err)

	graph, err := manifest.NewModuleGraph(pkg.Package.Modules.Modules)
	require.NoError(t, err)
	hashes := manifest.NewModuleHashes()
	for _, module := range pkg.Package.Modules.Modules {
		_, err := hashes.HashModule(pkg.Package.Modules, module, graph)
		require.NoError(t, err)
	}
	byName := map[string]string{}
	for _, module := range pkg.Package.Modules.Modules {
		byName[module.Name] = hashes.Get(module.Name)
	}

	return &thinstoreStack{
		t:        t,
		ctx:      ctx,
		endpoint: endpoint,
		pkg:      pkg.Package,
		hashes:   byName,
		stateDir: filepath.Join(tmpDir, "states"),
		last:     blocks,
	}
}

// run streams map_out over [start, stop) and returns the map_out id per block. A request
// still running after thinstoreQueryLimit is reported as hung with the goroutine dump of
// the (in-process) tier1.
func (s *thinstoreStack) run(start, stop uint64, productionMode bool) (map[uint64]string, error) {
	limit := thinstoreQueryLimit()
	ctx, cancel := context.WithTimeout(s.ctx, limit)
	defer cancel()

	conn, closeFunc, callOpts, _, err := client.NewSubstreamsClientConn(client.NewSubstreamsClientConfig(
		client.SubstreamsClientConfigOptions{
			Endpoint:  s.endpoint,
			AuthType:  client.None,
			PlainText: true,
			Agent:     "thinstore-test",
		},
	))
	if err != nil {
		return nil, err
	}
	defer closeFunc()

	blocks, err := pbsubstreamsrpcv4.NewStreamClient(conn).Blocks(ctx, &pbsubstreamsrpcv3.Request{
		StartBlockNum:  int64(start),
		StopBlockNum:   stop,
		ProductionMode: productionMode,
		OutputModule:   "map_out",
		Package:        s.pkg,
	}, callOpts...)
	if err != nil {
		return nil, err
	}

	out := map[uint64]string{}
	for {
		resp, err := blocks.Recv()
		if err != nil {
			if errors.Is(err, stream.ErrStopBlockReached) || errors.Is(err, io.EOF) {
				return out, nil
			}
			if ctx.Err() != nil {
				return nil, fmt.Errorf("%w after %s\n%s", errThinstoreHung, limit, thinstoreGoroutines())
			}
			return nil, err
		}
		if datas := resp.GetBlockScopedDatas(); datas != nil {
			for _, bsd := range datas.Items {
				out[bsd.Clock.Number] = thinstoreOutput(bsd)
			}
		}
	}
}

func thinstoreOutput(bsd *pbsubstreamsrpcv2.BlockScopedData) string {
	var clock pbsubstreams.Clock
	if bsd.Output == nil || bsd.Output.MapOutput == nil {
		return "<no output>"
	}
	if err := bsd.Output.MapOutput.UnmarshalTo(&clock); err != nil {
		return "<bad output: " + err.Error() + ">"
	}
	return clock.Id
}

// thinstoreGoroutines returns the substreams frames of the goroutine dump, which is where a
// hung request shows what it is waiting on since tier1 runs in the test process.
func thinstoreGoroutines() string {
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	var kept []string
	for _, g := range strings.Split(buf.String(), "\n\n") {
		if strings.Contains(g, "streamingfast/substreams/") && !strings.Contains(g, "thinstore_test.go") {
			kept = append(kept, g)
		}
	}
	return "== goroutines ==\n" + strings.Join(kept, "\n\n")
}

// check runs a query and compares every block with the baseline slice.
func (s *thinstoreStack) check(t *testing.T, start, stop uint64, productionMode bool) {
	t.Helper()
	got, err := s.run(start, stop, productionMode)
	require.NoError(t, err)
	s.expect(t, got, start, stop)
}

func (s *thinstoreStack) expect(t *testing.T, got map[uint64]string, start, stop uint64) {
	t.Helper()
	want := map[uint64]string{}
	for b, v := range s.baseline {
		if b >= start && b < stop {
			want[b] = v
		}
	}
	require.NotEmpty(t, want, "baseline has no block in [%d,%d)", start, stop)
	var missing, extra, differing []uint64
	for b := range want {
		if v, ok := got[b]; !ok {
			missing = append(missing, b)
		} else if v != want[b] {
			differing = append(differing, b)
		}
	}
	for b := range got {
		if _, ok := want[b]; !ok {
			extra = append(extra, b)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	sort.Slice(differing, func(i, j int) bool { return differing[i] < differing[j] })
	if len(missing)+len(extra)+len(differing) == 0 {
		return
	}
	msg := fmt.Sprintf("[%d,%d): baseline=%d got=%d missing=%v extra=%v differing=%v", start, stop, len(want), len(got), head(missing), head(extra), head(differing))
	for _, b := range head(differing) {
		msg += fmt.Sprintf("\n  block %d: want %q got %q", b, want[b], got[b])
	}
	require.Fail(t, "output differs from baseline", msg)
}

func head(in []uint64) []uint64 {
	if len(in) > 5 {
		return in[:5]
	}
	return in
}

// --- state store surgery --------------------------------------------------------------

// cacheFile is one state store file: the number is the file's end block for snapshots and
// its start block for outputs, partials and indexes (what the name starts with).
type cacheFile struct {
	path  string
	block uint64
	kind  string // kv, partial, output, index
}

func (s *thinstoreStack) files(module, folder string) []cacheFile {
	dir := filepath.Join(s.stateDir, s.hashes[module], folder)
	entries, _ := os.ReadDir(dir)
	var out []cacheFile
	for _, entry := range entries {
		m := thinstoreFileRegex.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		block, _ := strconv.ParseUint(m[1], 10, 64)
		out = append(out, cacheFile{path: filepath.Join(dir, entry.Name()), block: block, kind: m[3]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].block < out[j].block })
	return out
}

func (s *thinstoreStack) snapshots(module string) []cacheFile {
	var out []cacheFile
	for _, f := range s.files(module, "states") {
		if f.kind == "kv" {
			out = append(out, f)
		}
	}
	return out
}

func (s *thinstoreStack) storeModules() []string {
	var out []string
	for _, module := range s.pkg.Modules.Modules {
		if module.GetKindStore() != nil {
			out = append(out, module.Name)
		}
	}
	sort.Strings(out)
	return out
}

func (s *thinstoreStack) countFiles(folder string) int {
	n := 0
	for name := range s.hashes {
		n += len(s.files(name, folder))
	}
	return n
}

// prune is what `firecore tools substreams prune-states --keep-every N --keep-recent 1`
// does: every snapshot whose end block is not a multiple of keepEvery goes, except the
// most recent one of each store.
func (s *thinstoreStack) prune(keepEvery uint64) (deleted int) {
	for _, module := range s.storeModules() {
		snaps := s.snapshots(module)
		for i, f := range snaps {
			if i == len(snaps)-1 || f.block%keepEvery == 0 {
				continue
			}
			require.NoError(s.t, os.Remove(f.path))
			deleted++
		}
	}
	return deleted
}

// delSnapshots removes the fullKV files of `module` ending at the given blocks.
func (s *thinstoreStack) delSnapshots(module string, ends ...uint64) {
	for _, f := range s.snapshots(module) {
		for _, end := range ends {
			if f.block == end {
				require.NoError(s.t, os.Remove(f.path))
			}
		}
	}
	s.t.Logf("    %s: removed snapshots %v", module, ends)
}

// delFiles removes the `folder` (outputs, index) files of `module` starting in [from, to).
func (s *thinstoreStack) delFiles(module, folder string, from, to uint64) {
	for _, f := range s.files(module, folder) {
		if f.block >= from && f.block < to {
			require.NoError(s.t, os.Remove(f.path))
		}
	}
	s.t.Logf("    %s: removed %s [%d,%d)", module, folder, from, to)
}

// --- the suite --------------------------------------------------------------------------

func TestThinstore(t *testing.T) {
	if os.Getenv("SUBSTREAMS_E2E_THINSTORE") != "true" {
		t.Skip("set SUBSTREAMS_E2E_THINSTORE=true to run the thin store suite (~15 minutes, needs docker)")
	}
	blocks := uint64(thinstoreEnvInt("SUBSTREAMS_E2E_THINSTORE_BLOCKS", 15000))
	require.GreaterOrEqual(t, blocks, uint64(15000), "the scenarios punch holes up to block 13000")

	s := startThinstoreStack(t, blocks)

	t.Run("baseline", func(t *testing.T) {
		got, err := s.run(0, s.last, true)
		require.NoError(t, err)
		require.Len(t, got, int(s.last))
		s.baseline = got
		t.Logf("baseline: %d snapshots, %d outputs, %d index files", s.countFiles("states"), s.countFiles("outputs"), s.countFiles("index"))
	})
	require.NotNil(t, s.baseline, "baseline failed")

	t.Run("scenarios", s.scenarios)

	if iterations := thinstoreEnvInt("SUBSTREAMS_E2E_THINSTORE_FUZZ_ITERATIONS", 3); iterations > 0 {
		t.Run("fuzz", func(t *testing.T) { s.fuzz(t, iterations) })
	}
}

// scenarios prunes the snapshots, punches different holes per stage in snapshots,
// mapper outputs and index files, then replays requests over those holes.
func (s *thinstoreStack) scenarios(t *testing.T) {
	last := s.last
	t.Logf("prune: %d snapshots removed", s.prune(thinstoreKeepEvery))
	// stage 1
	s.delSnapshots("store_count", 4000, 8000)
	s.delFiles("map_a", "outputs", 0, 3000)
	s.delFiles("map_a", "outputs", 12000, 13000)
	s.delFiles("index_parity", "index", 1000, 2000)
	s.delFiles("index_parity", "index", 9000, 10000)
	// stage 2
	s.delSnapshots("store_sum", 3000, 4000, 8000)
	s.delSnapshots("store_a_last", 11000)
	s.delFiles("map_b", "outputs", 2000, 2500)
	s.delFiles("map_b", "outputs", 11000, 12000)
	s.delFiles("map_b", "outputs", last-2000, last-1000)
	// stage 3
	s.delSnapshots("store_c", 5000, 6000, 7000, 8000, 9000)
	s.delFiles("map_c", "outputs", 6000, 6500)
	s.delFiles("map_c", "outputs", 12500, last)
	// stage 4 (output module)
	s.delFiles("map_out", "outputs", 0, 2500)
	s.delFiles("map_out", "outputs", 3000, 3600)
	s.delFiles("map_out", "outputs", 5000, 8000)
	s.delFiles("map_out", "outputs", 9000, 9300)
	s.delFiles("map_out", "outputs", 11000, 11200)
	s.delFiles("map_out", "outputs", 12200, 12700)
	s.delFiles("map_out", "outputs", last-1000, last)
	t.Logf("after holes: %d snapshots, %d outputs, %d index files", s.countFiles("states"), s.countFiles("outputs"), s.countFiles("index"))

	cases := []struct {
		name        string
		start, stop uint64
		prod        bool
	}{
		// outputs kept near the head while older ones are gone: served from cache
		{"outputs_present_near_head", last - 3000, last - 2000, true},
		// outputs missing for the first part of the range, present after
		{"outputs_partially_present", 2300, 2700, true},
		// a 300-block output hole ending inside the range
		{"outputs_missing_then_present", 9200, 9500, true},
		// outputs present at the start, deleted from mid-range
		{"outputs_present_then_missing", 12000, 12400, true},
		// everything gone from genesis: resume from scratch
		{"genesis_all_missing", 0, 500, true},
		// index files and map_a/map_out outputs all missing
		{"index_hole_and_outputs_missing", 1100, 1300, true},
		// index deleted but final outputs present: nothing rebuilt
		{"index_hole_outputs_present", 9300, 9800, true},
		// stage-2 store missing 3000/4000: every store resumes at 2000
		{"stage2_holes", 3200, 3500, true},
		// stage-3 store missing 5000..9000 with map_c and map_out outputs gone
		{"stage3_big_hole", 6000, 6500, true},
		{"stage3_big_hole_to_kept", 7300, 8200, true},
		// stage-1 and stage-2 stores both missing 8000
		{"stage1_and_stage2_hole_at_8000", 7500, 8500, true},
		// store_a_last missing 11000 with map_b outputs deleted around it
		{"stage2_a_last_hole", 11000, 11300, true},
		// map_c and map_out outputs gone, stores intact
		{"upper_stages_missing", 12500, 12800, true},
		{"tail_all_missing", last - 1000, last, true},
		// dev mode over the same holes
		{"dev_stage3_big_hole", 6100, 6150, false},
		{"dev_stage2_a_last_hole", 11050, 11100, false},
		{"dev_tail", last - 50, last, false},
		// whole range again over every hole at once
		{"full_range", 0, last, true},
		// a rebuilt range must be served from what was just rebuilt
		{"rerun_stage3_big_hole", 6000, 6500, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			s.check(t, tc.start, tc.stop, tc.prod)
			t.Logf("[%d,%d) prod=%v took %s", tc.start, tc.stop, tc.prod, time.Since(start).Round(time.Millisecond))
		})
	}
	t.Logf("after scenarios: %d snapshots, %d outputs, %d index files", s.countFiles("states"), s.countFiles("outputs"), s.countFiles("index"))
}

// --- fuzz -------------------------------------------------------------------------------

type fuzzQuery struct {
	start, stop uint64
	prod        bool
}

func (q fuzzQuery) String() string {
	mode := "dev "
	if q.prod {
		mode = "prod"
	}
	return fmt.Sprintf("%s [%d,%d)", mode, q.start, q.stop)
}

// fuzz turns the state store into gruyère with a random deletion plan per module and file
// kind, then runs random queries biased towards the hole edges and compares with the
// baseline. With chaos, files keep being deleted while the queries run: a query that lost
// a file it needed and failed with the pruned-snapshot error is a casualty, not a failure.
func (s *thinstoreStack) fuzz(t *testing.T, iterations int) {
	seed := int64(thinstoreEnvInt("SUBSTREAMS_E2E_THINSTORE_FUZZ_SEED", 1))
	chaos := os.Getenv("SUBSTREAMS_E2E_THINSTORE_FUZZ_CHAOS") == "true"
	rng := rand.New(rand.NewSource(seed))
	t.Logf("fuzz seed=%d iterations=%d chaos=%v", seed, iterations, chaos)

	for it := 0; it < iterations; it++ {
		t.Run(fmt.Sprintf("iteration_%d", it), func(t *testing.T) {
			edges := s.gruyere(t, rng)
			queries := make([]fuzzQuery, 5)
			for i := range queries {
				queries[i] = s.randomQuery(rng, edges)
			}

			stop := make(chan struct{})
			var chaosWg sync.WaitGroup
			var chaosDeleted int
			if chaos {
				chaosWg.Add(1)
				go func() {
					defer chaosWg.Done()
					chaosDeleted = s.chaosLoop(rng, stop)
				}()
			}
			for _, q := range queries {
				started := time.Now()
				got, err := s.run(q.start, q.stop, q.prod)
				elapsed := time.Since(started).Round(time.Millisecond)
				switch {
				case err != nil && chaos && (strings.Contains(err.Error(), thinstoreChaosPruned) || strings.Contains(err.Error(), "opening file for streaming: not found")):
					t.Logf("  ~ %s %s: lost a file to chaos, failed cleanly", q, elapsed)
				case errors.Is(err, errThinstoreHung):
					t.Fatalf("  ✗ %s %s (seed %d): %v", q, elapsed, seed, err)
				case err != nil:
					t.Errorf("  ✗ %s %s (seed %d): %v", q, elapsed, seed, err)
				default:
					s.expect(t, got, q.start, q.stop)
					t.Logf("  ✓ %s %s", q, elapsed)
				}
			}
			close(stop)
			chaosWg.Wait()
			if chaos {
				t.Logf("  chaos deleted %d files while queries ran", chaosDeleted)
			}
		})
	}
}

// gruyere applies one random deletion strategy per module and file kind and returns the
// block numbers at the edges of the holes, for the query generator to aim at.
func (s *thinstoreStack) gruyere(t *testing.T, rng *rand.Rand) (edges []uint64) {
	edges = []uint64{0, s.last}
	pick := func(weights map[string]int) string {
		total := 0
		for _, w := range weights {
			total += w
		}
		n := rng.Intn(total)
		keys := make([]string, 0, len(weights))
		for k := range weights {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n -= weights[k]
			if n < 0 {
				return k
			}
		}
		return keys[0]
	}

	names := make([]string, 0, len(s.hashes))
	for name := range s.hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, module := range names {
		for _, folder := range []string{"states", "outputs", "index"} {
			byKind := map[string][]cacheFile{}
			for _, f := range s.files(module, folder) {
				byKind[f.kind] = append(byKind[f.kind], f)
			}
			for _, kind := range []string{"kv", "partial", "output", "index"} {
				files := byKind[kind]
				if len(files) == 0 {
					continue
				}
				var strategy string
				switch kind {
				case "kv":
					strategy = pick(map[string]int{"random": 4, "keep_every": 4, "contiguous": 3, "all_below": 2, "all_but_few": 2, "everything": 2, "odd": 2, "none": 2})
				case "partial":
					strategy = pick(map[string]int{"random": 1, "everything": 1, "none": 1})
				default:
					strategy = pick(map[string]int{"random": 4, "contiguous": 3, "all_below": 2, "everything": 1, "none": 3})
				}
				var doomed []cacheFile
				switch strategy {
				case "random":
					p := []float64{0.1, 0.5, 0.9, 0.98}[rng.Intn(4)]
					for _, f := range files {
						if rng.Float64() < p {
							doomed = append(doomed, f)
						}
					}
				case "keep_every":
					every := []uint64{300, 1000, 2000, 5000, 7700}[rng.Intn(5)]
					for _, f := range files {
						if f.block%every != 0 {
							doomed = append(doomed, f)
						}
					}
				case "contiguous":
					span := []uint64{300, 1000, 5000, 15000}[rng.Intn(4)]
					from := files[rng.Intn(len(files))].block
					for _, f := range files {
						if f.block >= from && f.block < from+span {
							doomed = append(doomed, f)
						}
					}
				case "all_below":
					below := uint64(rng.Intn(int(s.last/thinstoreSegment))) * thinstoreSegment
					for _, f := range files {
						if f.block <= below {
							doomed = append(doomed, f)
						}
					}
				case "all_but_few":
					keep := map[int]bool{}
					for n := rng.Intn(3) + 1; n > 0; n-- {
						keep[rng.Intn(len(files))] = true
					}
					for i, f := range files {
						if !keep[i] {
							doomed = append(doomed, f)
						}
					}
				case "everything":
					doomed = files
				case "odd": // stores end up with no common kept boundary
					every := []uint64{1000, 2000}[rng.Intn(2)]
					for _, f := range files {
						if f.block%every == 0 {
							doomed = append(doomed, f)
						}
					}
				}
				for _, f := range doomed {
					os.Remove(f.path)
					if len(edges) < 400 {
						edges = append(edges, f.block)
					}
				}
				if len(doomed) > 0 {
					t.Logf("  %s/%s: %s (%d gone, %d left)", module, kind, strategy, len(doomed), len(files)-len(doomed))
				}
			}
		}
	}
	return edges
}

// randomQuery biases ranges towards hole edges (±1, ±100 blocks), tiny, long and tail ranges.
func (s *thinstoreStack) randomQuery(rng *rand.Rand, edges []uint64) fuzzQuery {
	var start, length uint64
	switch n := rng.Intn(12); {
	case n < 5: // edge
		offsets := []int64{-150, -101, -100, -1, 0, 1, 50, 99, 100, 101}
		base := int64(edges[rng.Intn(len(edges))]) + offsets[rng.Intn(len(offsets))]
		if base < 0 {
			base = 0
		}
		start = uint64(base)
		length = []uint64{1, 2, 50, 100, 101, 250, 1000}[rng.Intn(7)]
	case n < 8: // uniform
		start = uint64(rng.Intn(int(s.last)))
		length = []uint64{10, 100, 300, 1000, 2500}[rng.Intn(5)]
	case n < 9: // long
		start = uint64(rng.Intn(int(s.last / 2)))
		length = 3000 + uint64(rng.Intn(int(s.last/2)))
	case n < 11: // tiny
		start = uint64(rng.Intn(int(s.last)))
		length = 1
	default: // tail
		start = s.last - []uint64{1, 10, 100, 350, 1000}[rng.Intn(5)]
		length = s.last
	}
	stop := start + length
	if stop > s.last {
		stop = s.last
	}
	if stop <= start {
		start, stop = s.last-100, s.last
	}
	return fuzzQuery{start: start, stop: stop, prod: rng.Intn(4) != 0}
}

// chaosLoop deletes a few random cache files every couple of seconds until stopped: slow
// enough that some queries still complete, fast enough that others lose a file mid-flight.
func (s *thinstoreStack) chaosLoop(rng *rand.Rand, stop <-chan struct{}) (deleted int) {
	for {
		select {
		case <-stop:
			return deleted
		case <-time.After(time.Duration(1500+rng.Intn(2500)) * time.Millisecond):
		}
		var victims []cacheFile
		for name := range s.hashes {
			for _, folder := range []string{"states", "outputs", "index"} {
				victims = append(victims, s.files(name, folder)...)
			}
		}
		if len(victims) == 0 {
			continue
		}
		for n := []int{1, 2, 5}[rng.Intn(3)]; n > 0 && len(victims) > 0; n-- {
			i := rng.Intn(len(victims))
			if os.Remove(victims[i].path) == nil {
				deleted++
			}
			victims = append(victims[:i], victims[i+1:]...)
		}
	}
}
