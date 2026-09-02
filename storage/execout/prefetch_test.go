package execout

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// prefetchStore is a MockStore safe to share between the walker and the prefetch
// goroutines, that reports a missing file the way real stores do, records which
// files were opened, and fails the test on any attributes lookup.
type prefetchStore struct {
	*dstore.MockStore
	mu     sync.Mutex
	opened []string
}

func newPrefetchStore(t *testing.T) *prefetchStore {
	s := &prefetchStore{MockStore: dstore.NewMockStore(nil)}
	s.MockStore.OpenObjectFunc = func(_ context.Context, name string) (io.ReadCloser, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		content, found := s.Files[name]
		if !found {
			return nil, dstore.ErrNotFound
		}
		s.opened = append(s.opened, name)
		return io.NopCloser(bytes.NewReader(content)), nil
	}
	s.MockStore.ObjectAttributesFunc = func(_ context.Context, name string) (*dstore.ObjectAttributes, error) {
		t.Errorf("unexpected attributes lookup of %q: the prefetcher must never ask the store for sizes", name)
		return nil, fmt.Errorf("unexpected attributes lookup")
	}
	return s
}

func (s *prefetchStore) openedFiles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.opened...)
}

// writeSegment writes one output file holding one item per block of rng, each with a
// payload of payloadSize bytes.
func (s *prefetchStore) writeSegment(t *testing.T, rng *block.Range, payloadSize int) {
	t.Helper()
	store := dstore.NewMockStore(nil)
	store.SetMetadataFunc = func(context.Context, string, map[string]string) error { return nil }
	fw := NewFileWriter(context.Background(), store, zap.NewNop(), rng, "mod")
	for num := rng.StartBlock; num < rng.ExclusiveEndBlock; num++ {
		payload := bytes.Repeat([]byte{byte(num)}, payloadSize)
		require.NoError(t, fw.SetItem(&pbsubstreams.Clock{Number: num, Id: fmt.Sprintf("block-%d", num)}, payload))
	}
	require.NoError(t, fw.Close())

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Files[fw.Filename()] = store.Files[fw.Filename()]
}

func newPrefetchWalker(store *prefetchStore, segmenter *block.Segmenter, cfg PrefetchConfig) *FileWalker {
	config := &Config{name: "mod", objStore: store, logger: zap.NewNop()}
	return NewFileWalker(config, segmenter, zap.NewNop()).WithPrefetch(cfg)
}

// readCurrent streams the walker's current segment, appends its block numbers to
// blocks and advances the walker.
func readCurrent(t *testing.T, ctx context.Context, walker *FileWalker, blocks *[]uint64) error {
	t.Helper()
	reader, err := walker.FileReader(ctx)
	if err != nil {
		return err
	}
	for item, err := range reader.Iter() {
		require.NoError(t, err)
		*blocks = append(*blocks, item.BlockNum)
	}
	require.NoError(t, reader.Close())
	walker.Next()
	return nil
}

func walkAll(t *testing.T, ctx context.Context, walker *FileWalker) []uint64 {
	t.Helper()
	var blocks []uint64
	for !walker.IsDone() {
		require.NoError(t, readCurrent(t, ctx, walker, &blocks))
	}
	return blocks
}

func blockNums(from, to uint64) (out []uint64) {
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}

func filename(segmenter *block.Segmenter, segment int) string {
	rng := segmenter.Range(segment)
	return computeDBinFilename(rng.StartBlock, rng.ExclusiveEndBlock)
}

func countOf(list []string, value string) (n int) {
	for _, v := range list {
		if v == value {
			n++
		}
	}
	return n
}

func assertNothingHeld(t *testing.T, p *prefetcher) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	assert.Zero(t, p.held, "every segment released its bytes")
	assert.Empty(t, p.segments)
}

func TestPrefetch_StreamsEverySegmentInOrder(t *testing.T) {
	store := newPrefetchStore(t)
	segmenter := block.NewSegmenter(10, 0, 60)
	for i := 0; i < 6; i++ {
		store.writeSegment(t, segmenter.Range(i), 8)
	}
	walker := newPrefetchWalker(store, segmenter, PrefetchConfig{Depth: 3, BudgetBytes: 1 << 20})

	ctx := context.Background()
	first, err := walker.FileReader(ctx)
	require.NoError(t, err)

	// Segment 1 is probed alone, then its size lets 2 and 3 download together, all
	// while segment 0 is still being read.
	require.Eventually(t, func() bool { return len(store.openedFiles()) == 4 }, 2*time.Second, time.Millisecond)
	assert.Equal(t, filename(segmenter, 1), store.openedFiles()[1])
	assert.ElementsMatch(t, []string{filename(segmenter, 2), filename(segmenter, 3)}, store.openedFiles()[2:])
	require.NoError(t, first.Close())
	walker.Next()

	var blocks []uint64
	for !walker.IsDone() {
		require.NoError(t, readCurrent(t, ctx, walker, &blocks))
	}
	assert.Equal(t, blockNums(10, 60), blocks)
	assert.Len(t, store.openedFiles(), 6, "every segment was opened exactly once")
	assertNothingHeld(t, walker.prefetch)
}

func TestPrefetch_DepthIsCapped(t *testing.T) {
	walker := newPrefetchWalker(newPrefetchStore(t), block.NewSegmenter(10, 0, 100), PrefetchConfig{Depth: 50, BudgetBytes: 1 << 20})
	assert.Equal(t, MaxPrefetchDepth, walker.prefetch.cfg.Depth)
}

func TestPrefetch_BudgetBoundsHowManySegmentsRunAhead(t *testing.T) {
	store := newPrefetchStore(t)
	segmenter := block.NewSegmenter(10, 0, 60)
	for i := 0; i < 6; i++ {
		store.writeSegment(t, segmenter.Range(i), 8)
	}
	segmentSize := uint64(len(store.Files[filename(segmenter, 1)]))
	// Room for two segments and a bit, so at most two are ever ahead of the walker.
	walker := newPrefetchWalker(store, segmenter, PrefetchConfig{Depth: 4, BudgetBytes: segmentSize*2 + segmentSize/2})

	ctx := context.Background()
	first, err := walker.FileReader(ctx)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(store.openedFiles()) == 3 }, 2*time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	assert.Len(t, store.openedFiles(), 3, "segment 3 waits for the budget")
	require.NoError(t, first.Close())
	walker.Next()

	var blocks []uint64
	for !walker.IsDone() {
		require.NoError(t, readCurrent(t, ctx, walker, &blocks))
	}
	assert.Equal(t, blockNums(10, 60), blocks)
	assertNothingHeld(t, walker.prefetch)
}

func TestPrefetch_OverflowTurnsPrefetchingOff(t *testing.T) {
	store := newPrefetchStore(t)
	segmenter := block.NewSegmenter(10, 0, 40)
	store.writeSegment(t, segmenter.Range(0), 8)
	store.writeSegment(t, segmenter.Range(1), 100) // over the budget
	store.writeSegment(t, segmenter.Range(2), 8)
	store.writeSegment(t, segmenter.Range(3), 8)
	walker := newPrefetchWalker(store, segmenter, PrefetchConfig{Depth: 4, BudgetBytes: 500})

	ctx := context.Background()
	first, err := walker.FileReader(ctx)
	require.NoError(t, err)
	p := walker.prefetch
	require.Eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.disabled
	}, 2*time.Second, time.Millisecond, "the probe of segment 1 overflows and turns prefetching off")
	require.NoError(t, first.Close())
	walker.Next()

	blocks := blockNums(0, 10)
	for !walker.IsDone() {
		require.NoError(t, readCurrent(t, ctx, walker, &blocks))
	}
	assert.Equal(t, blockNums(0, 40), blocks)

	// Segment 1 was probed by the prefetcher, dropped, then opened by the walker.
	// Nothing after it was prefetched.
	opened := store.openedFiles()
	assert.Equal(t, 2, countOf(opened, filename(segmenter, 1)))
	assert.Equal(t, 1, countOf(opened, filename(segmenter, 2)))
	assert.Equal(t, 1, countOf(opened, filename(segmenter, 3)))
	assertNothingHeld(t, p)
}

func TestPrefetch_MissingSegmentIsLeftToTheWalker(t *testing.T) {
	store := newPrefetchStore(t)
	segmenter := block.NewSegmenter(10, 0, 80)
	for i := 0; i < 8; i++ {
		if i != 2 {
			store.writeSegment(t, segmenter.Range(i), 8)
		}
	}
	walker := newPrefetchWalker(store, segmenter, PrefetchConfig{Depth: 4, BudgetBytes: 1 << 20})

	ctx := context.Background()
	var blocks []uint64
	require.NoError(t, readCurrent(t, ctx, walker, &blocks))
	require.NoError(t, readCurrent(t, ctx, walker, &blocks))

	// Segment 2 failed to open. Up to a depth's worth of segments after it may already
	// have been launched alongside it and are held, but the launcher then stops until
	// the walker reaches the gap, so the far end of the range is never touched.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, countOf(store.openedFiles(), filename(segmenter, 7)))

	require.ErrorIs(t, readCurrent(t, ctx, walker, &blocks), dstore.ErrNotFound)
	store.writeSegment(t, segmenter.Range(2), 8)
	for !walker.IsDone() {
		require.NoError(t, readCurrent(t, ctx, walker, &blocks))
	}
	assert.Equal(t, blockNums(0, 80), blocks)
	assert.Equal(t, 1, countOf(store.openedFiles(), filename(segmenter, 7)), "the far end was prefetched once the walker passed the gap")
	assertNothingHeld(t, walker.prefetch)
}

func TestPrefetch_CancelStopsTheGoroutine(t *testing.T) {
	store := newPrefetchStore(t)
	segmenter := block.NewSegmenter(10, 0, 30)
	for i := 0; i < 3; i++ {
		store.writeSegment(t, segmenter.Range(i), 8)
	}
	walker := newPrefetchWalker(store, segmenter, PrefetchConfig{Depth: 1, BudgetBytes: 1 << 20})

	ctx, cancel := context.WithCancel(context.Background())
	reader, err := walker.FileReader(ctx)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	// Depth 1 keeps the launcher waiting on segment 1's release before it can fetch 2.
	require.Eventually(t, func() bool { return countOf(store.openedFiles(), filename(segmenter, 1)) == 1 }, 2*time.Second, time.Millisecond)
	cancel()

	p := walker.prefetch
	require.Eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.done
	}, 2*time.Second, time.Millisecond)
	assert.Equal(t, 0, countOf(store.openedFiles(), filename(segmenter, 2)))
}
