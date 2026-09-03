package execout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap"
)

// MaxPrefetchDepth is the most segments a walker ever holds or downloads ahead,
// whatever PrefetchConfig.Depth says.
const MaxPrefetchDepth = 4

// PrefetchConfig bounds how far ahead of the segment being streamed a FileWalker
// downloads execution output files, and how much decompressed data it may hold
// in memory while doing so. A zero Depth or BudgetBytes disables prefetching.
type PrefetchConfig struct {
	// Depth is the number of segments that may be held or in flight at once,
	// capped at MaxPrefetchDepth.
	Depth int
	// BudgetBytes is the total decompressed size of the segments held in memory.
	BudgetBytes uint64
}

func (c PrefetchConfig) enabled() bool {
	return c.Depth > 0 && c.BudgetBytes > 0
}

// prefetcher downloads the segments following the one the walker is streaming, so
// the store round trips and the decompression of the next segments overlap with
// sending the current one. The walker asks it for each segment in order; a segment
// the prefetcher does not hold (never reached, skipped, or failed) is opened
// directly by the walker.
//
// It never asks the store for a file's size. The decompressed size of the last
// completed download is the estimate for the next ones, and as many downloads run
// at once as estimate-sized files fit in the budget, at least one and at most
// Depth. The budget is split evenly between them: each in-flight download reserves
// its share and may read no more than that, so in-flight reads and held segments
// never add up to more than the budget. Until a first download completes there is
// no estimate, so that one runs alone against the whole budget.
//
// A file bigger than its share is dropped and left to the walker, and the estimate
// doubles so fewer downloads share the budget next time. A file bigger than the
// whole budget turns prefetching off for the rest of the request.
//
// A download that fails, including on a file tier2 has not written yet, is simply
// dropped: the walker's own retry loop owns waiting for files to appear. The
// prefetcher stops launching further segments until the walker has reached the
// dropped one, then resumes right after it.
type prefetcher struct {
	cfg    PrefetchConfig
	config *Config
	walker *FileWalker
	logger *zap.Logger

	mu       sync.Mutex
	cond     *sync.Cond
	started  bool
	segments map[int]*prefetchedSegment
	held     uint64 // shares reserved by in-flight downloads plus bytes held by completed ones, never above the budget
	estimate uint64 // decompressed size of the last completed download, raised by overflows, 0 until one completes
	consumed int    // highest segment index the walker has asked for
	failed   int    // lowest segment whose download failed and the walker has not reached, -1 if none
	disabled bool   // a download overflowed the budget: no further segment is started
	done     bool   // the launching goroutine has exited
}

type prefetchedSegment struct {
	reserved uint64 // bytes counted against the budget
	data     []byte
	err      error
	ready    chan struct{}
}

var errPrefetchOverflow = errors.New("execout file exceeds the prefetch budget")

func newPrefetcher(cfg PrefetchConfig, walker *FileWalker) *prefetcher {
	cfg.Depth = min(cfg.Depth, MaxPrefetchDepth)
	p := &prefetcher{
		cfg:      cfg,
		config:   walker.config,
		walker:   walker,
		logger:   walker.logger.Named("execout_prefetch"),
		segments: make(map[int]*prefetchedSegment),
		consumed: walker.segmenter.FirstIndex() - 1,
		failed:   -1,
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// take hands the walker the segment it asks for, waiting for it if it is still
// downloading. It returns (nil, nil) when the walker has to open the segment itself.
func (p *prefetcher) take(ctx context.Context, segment int) (FileReader, error) {
	p.mu.Lock()
	if segment > p.consumed {
		p.consumed = segment
	}
	if !p.started {
		p.started = true
		go p.run(ctx, segment+1)
	}
	seg := p.segments[segment]
	p.cond.Broadcast()
	p.mu.Unlock()

	if seg == nil {
		return nil, nil
	}

	select {
	case <-seg.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if seg.err != nil {
		return nil, nil
	}

	return &fileReader{
		File:   p.config.NewFile(p.walker.segmenter.Range(segment)),
		reader: &releaseOnClose{Reader: bytes.NewReader(seg.data), release: func() { p.release(segment) }},
	}, nil
}

func (p *prefetcher) release(segment int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releaseLocked(segment)
	p.cond.Broadcast()
}

func (p *prefetcher) releaseLocked(segment int) {
	if seg, found := p.segments[segment]; found {
		p.held -= seg.reserved
		delete(p.segments, segment)
	}
}

// run launches downloads in segment order, each in its own goroutine, whenever the
// budget and Depth allow one more.
func (p *prefetcher) run(ctx context.Context, from int) {
	defer func() {
		p.mu.Lock()
		p.done = true
		p.cond.Broadcast()
		p.mu.Unlock()
	}()

	stop := context.AfterFunc(ctx, func() {
		p.mu.Lock()
		p.cond.Broadcast()
		p.mu.Unlock()
	})
	defer stop()

	last := p.walker.segmenter.LastIndex()
	next := from

	// Picking the segment and reserving it happen under one hold of the lock, so the
	// walker cannot ask for that segment in between and end up opening it itself
	// while the download is launched anyway.
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		for !p.launchableLocked(&next, last) {
			if ctx.Err() != nil {
				return
			}
			p.cond.Wait()
		}
		if p.disabled || next > last {
			return
		}
		segment := next
		rng := p.walker.segmenter.Range(segment)
		if rng == nil {
			return
		}
		seg, limit := p.reserveLocked(segment)
		next++

		p.mu.Unlock()
		go func() {
			data, err := p.download(ctx, rng, limit)
			p.complete(segment, seg, data, err)
		}()
		p.mu.Lock()
	}
}

// launchableLocked advances next past segments the walker already asked for or that
// are already held, and says whether a download may be launched now, or whether
// there is nothing left to do (next past last with nothing held, or disabled).
func (p *prefetcher) launchableLocked(next *int, last int) bool {
	if p.disabled {
		return true
	}
	if p.failed >= 0 {
		if p.consumed < p.failed {
			return false
		}
		*next = min(*next, p.failed+1)
		p.failed = -1
	}
	for *next <= p.consumed || p.segments[*next] != nil {
		*next++
	}
	if *next > last {
		return len(p.segments) == 0
	}
	return p.canStartLocked()
}

// concurrencyLocked is how many downloads may run at once: as many estimate-sized
// files as fit in the budget, at least one, at most Depth. Without an estimate,
// one.
func (p *prefetcher) concurrencyLocked() int {
	if p.estimate == 0 {
		return 1
	}
	return int(max(1, min(uint64(p.cfg.Depth), p.cfg.BudgetBytes/p.estimate)))
}

// shareLocked is the most one download may read: the budget split evenly between
// the downloads allowed to run at once.
func (p *prefetcher) shareLocked() uint64 {
	return p.cfg.BudgetBytes / uint64(p.concurrencyLocked())
}

// canStartLocked says whether one more download may begin: a slot is free and its
// full share still fits next to what is held.
func (p *prefetcher) canStartLocked() bool {
	return len(p.segments) < p.concurrencyLocked() && p.held+p.shareLocked() <= p.cfg.BudgetBytes
}

// reserveLocked registers the segment as in flight, holding its whole share until
// the download completes, and returns that share as the read limit.
func (p *prefetcher) reserveLocked(segment int) (seg *prefetchedSegment, limit uint64) {
	limit = p.shareLocked()
	seg = &prefetchedSegment{reserved: limit, ready: make(chan struct{})}
	p.segments[segment] = seg
	p.held += limit
	return seg, limit
}

func (p *prefetcher) complete(segment int, seg *prefetchedSegment, data []byte, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	seg.data = data
	seg.err = err
	switch {
	case err == nil:
		p.held -= seg.reserved
		seg.reserved = uint64(len(data))
		p.held += seg.reserved
		p.estimate = max(seg.reserved, 1)
	case errors.Is(err, errPrefetchOverflow) && seg.reserved >= p.cfg.BudgetBytes:
		p.releaseLocked(segment)
		p.disabled = true
		p.logger.Info("execout prefetching turned off for this request: a file exceeds the budget",
			zap.Int("segment", segment), zap.Uint64("budget", p.cfg.BudgetBytes))
	case errors.Is(err, errPrefetchOverflow):
		p.releaseLocked(segment)
		p.estimate = max(p.estimate, min(2*seg.reserved, p.cfg.BudgetBytes))
		if p.failed < 0 || segment < p.failed {
			p.failed = segment
		}
		p.logger.Debug("execout file exceeds its share of the prefetch budget, walker will open it directly",
			zap.Int("segment", segment), zap.Uint64("share", seg.reserved), zap.Uint64("new_estimate", p.estimate))
	default:
		p.releaseLocked(segment)
		if p.failed < 0 || segment < p.failed {
			p.failed = segment
		}
		if !errors.Is(err, dstore.ErrNotFound) {
			p.logger.Debug("execout file not prefetched, walker will open it directly", zap.Int("segment", segment), zap.Error(err))
		}
	}
	close(seg.ready)
	p.cond.Broadcast()
}

// download reads the whole segment, decompressed, into memory. It fails with
// errPrefetchOverflow when the file does not fit in limit bytes.
func (p *prefetcher) download(ctx context.Context, rng *block.Range, limit uint64) ([]byte, error) {
	fr := &fileReader{File: p.config.NewFile(rng)}
	if err := fr.open(ctx); err != nil {
		return nil, err
	}
	defer fr.reader.Close()

	data, err := io.ReadAll(io.LimitReader(fr.reader, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) > limit {
		return nil, fmt.Errorf("%w: more than %d bytes", errPrefetchOverflow, limit)
	}
	return data, nil
}

type releaseOnClose struct {
	io.Reader
	release func()
	once    sync.Once
}

func (r *releaseOnClose) Close() error {
	r.once.Do(r.release)
	return nil
}
