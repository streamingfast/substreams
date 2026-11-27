// Package file registers the `file://` emitter: change events written as JSON lines to files rotated every
// N blocks. Importing the package registers it.
//
// The emitter writes to the local filesystem only. Object stores (gs://, s3://) have no append nor atomic
// rename, so they would need a different durability design.
package file

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/streamingfast/substreams/sink/cdc"
	"github.com/streamingfast/substreams/sink/cdc/emitters/params"
	"go.uber.org/zap"
)

// DefaultBlocks is the default number of blocks a file spans.
const DefaultBlocks = 1000

const (
	partSuffix  = ".part"
	finalSuffix = ".jsonl"
)

func init() {
	cdc.RegisterEmitter("file", func(_ context.Context, u *url.URL, opts cdc.EmitterOptions) (cdc.Emitter, error) {
		q := params.New(u)
		blocks := q.Int("blocks", DefaultBlocks)
		cfg := Config{
			Dir:      filepath.Join(u.Host, filepath.FromSlash(u.Path)),
			Blocks:   uint64(max(blocks, 0)),
			Prefix:   q.String("prefix", ""),
			Finalize: q.Bool("finalize", false),
		}
		if err := q.Err(); err != nil {
			return nil, err
		}
		if unused := q.Unused(); len(unused) > 0 {
			return nil, fmt.Errorf("unknown file:// parameters %s, supported: blocks, prefix, finalize", unused.Encode())
		}
		if blocks <= 0 {
			return nil, errors.New("file:// parameter blocks must be greater than 0")
		}
		return New(cfg, opts.Logger)
	})
}

// Config configures an [Emitter].
type Config struct {
	// Dir is the output directory, created when missing.
	Dir string
	// Blocks is how many blocks a file spans, files cover aligned ranges [k*Blocks, (k+1)*Blocks).
	Blocks uint64
	// Prefix is prepended to every file name.
	Prefix string
	// Finalize turns the file being written into a final one on Close even when its range is not complete, named
	// after the blocks it actually holds. By default it stays a `.part` file that the next run continues.
	Finalize bool
}

// Emitter appends JSON lines to the file of the current block range.
//
// The file of the range being filled is `<prefix><start>-<end>.part`, with start and end the aligned,
// inclusive, zero-padded range bounds. It is fsynced after each batch, so a batch is durable when
// EmitBatch returns, and renamed to `.jsonl` once the stream moves past its range, so a `.jsonl` file is
// complete and never changes. After a crash the `.part` file is picked up again; a batch emitted twice
// shows up twice, ids are deterministic to deduplicate.
//
// Compensation events are appended to the current file like any other event: the files are an append-only
// log, a reader applies the `revert_of` tombstones itself.
type Emitter struct {
	cfg    Config
	logger *zap.Logger

	file       *os.File
	rangeStart uint64
	hasBlock   bool
	// adopted is true when the open file comes from a previous run, so first/last do not cover all of it.
	adopted bool
	// first and last are the lowest and highest block numbers written to the open file.
	first, last uint64
	buf         []byte
}

var _ cdc.Emitter = (*Emitter)(nil)

// New creates the emitter, picking up a leftover `.part` file when there is one.
func New(cfg Config, logger *zap.Logger) (*Emitter, error) {
	if cfg.Blocks == 0 {
		cfg.Blocks = DefaultBlocks
	}
	if cfg.Dir == "" {
		return nil, errors.New("file output needs a directory, e.g. file://./out")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	e := &Emitter{cfg: cfg, logger: logger}
	if err := e.adoptPart(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Emitter) rangeName(start uint64) string {
	return fmt.Sprintf("%s%010d-%010d", e.cfg.Prefix, start, start+e.cfg.Blocks-1)
}

func (e *Emitter) partPath(start uint64) string {
	return filepath.Join(e.cfg.Dir, e.rangeName(start)+partSuffix)
}

// adoptPart resumes writing in the `.part` file left by a previous run, the highest one when several.
func (e *Emitter) adoptPart() error {
	entries, err := os.ReadDir(e.cfg.Dir)
	if err != nil {
		return fmt.Errorf("read output directory: %w", err)
	}

	var latest string
	var latestStart uint64
	for _, entry := range entries {
		name := entry.Name()
		rest, ok := strings.CutPrefix(name, e.cfg.Prefix)
		if !ok || !strings.HasSuffix(name, partSuffix) {
			continue
		}
		var start, end uint64
		if _, err := fmt.Sscanf(strings.TrimSuffix(rest, partSuffix), "%d-%d", &start, &end); err != nil {
			continue
		}
		if end-start+1 != e.cfg.Blocks {
			return fmt.Errorf("found %q which spans %d blocks but the emitter is configured for %d, use another directory or prefix", name, end-start+1, e.cfg.Blocks)
		}
		if latest == "" || start > latestStart {
			latest, latestStart = name, start
		}
	}
	if latest == "" {
		return nil
	}

	f, err := os.OpenFile(filepath.Join(e.cfg.Dir, latest), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("reopen %q: %w", latest, err)
	}
	e.file, e.rangeStart, e.hasBlock, e.adopted = f, latestStart, false, true
	e.logger.Info("continuing leftover part file", zap.String("file", latest))
	return nil
}

func (e *Emitter) EmitBatch(_ context.Context, batch *cdc.Batch) error {
	start := batch.Block.Number / e.cfg.Blocks * e.cfg.Blocks
	if e.file != nil && start != e.rangeStart && start > e.rangeStart {
		if err := e.rotate(); err != nil {
			return err
		}
	}
	// A batch below the open range (restart after an undo) goes to the open file: the files are a log.
	return e.write(batch, start)
}

// EmitCompensation appends to the open file, it never rotates: its block is the last valid one, which is
// below the range being written.
func (e *Emitter) EmitCompensation(_ context.Context, batch *cdc.Batch) error {
	return e.write(batch, batch.Block.Number/e.cfg.Blocks*e.cfg.Blocks)
}

func (e *Emitter) write(batch *cdc.Batch, start uint64) error {
	e.buf = e.buf[:0]
	for i := range batch.Events {
		var err error
		if e.buf, err = batch.Events[i].AppendJSONL(e.buf); err != nil {
			return err
		}
	}
	if len(e.buf) == 0 {
		return nil
	}

	if e.file == nil {
		f, err := os.OpenFile(e.partPath(start), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("open part file: %w", err)
		}
		e.file, e.rangeStart, e.hasBlock, e.adopted = f, start, false, false
	}

	if _, err := e.file.Write(e.buf); err != nil {
		return fmt.Errorf("write %q: %w", e.file.Name(), err)
	}
	if err := e.file.Sync(); err != nil {
		return fmt.Errorf("fsync %q: %w", e.file.Name(), err)
	}

	n := batch.Block.Number
	if !e.hasBlock {
		e.first, e.last, e.hasBlock = n, n, true
	} else {
		e.first, e.last = min(e.first, n), max(e.last, n)
	}
	return nil
}

// rotate completes the open file, fsyncing the rename through the directory.
func (e *Emitter) rotate() error {
	finalPath := filepath.Join(e.cfg.Dir, e.rangeName(e.rangeStart)+finalSuffix)
	return e.finish(finalPath)
}

func (e *Emitter) finish(finalPath string) error {
	f := e.file
	e.file = nil
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("fsync %q: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %q: %w", f.Name(), err)
	}
	if err := os.Rename(f.Name(), finalPath); err != nil {
		return fmt.Errorf("rename %q: %w", f.Name(), err)
	}
	e.logger.Info("completed file", zap.String("file", filepath.Base(finalPath)))
	return syncDir(e.cfg.Dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	// Directory fsync is not supported everywhere, the rename itself is already atomic.
	_ = d.Sync()
	return nil
}

// Flush is a no-op, every batch is fsynced when emitted.
func (e *Emitter) Flush(context.Context) error { return nil }

// Close closes the open file. It stays a `.part` file, unless [Config.Finalize] is set and the file holds
// events from this run only, then it is renamed after the blocks it holds.
func (e *Emitter) Close() error {
	if e.file == nil {
		return nil
	}
	if e.cfg.Finalize && e.hasBlock && !e.adopted {
		return e.finish(filepath.Join(e.cfg.Dir, fmt.Sprintf("%s%010d-%010d%s", e.cfg.Prefix, e.first, e.last, finalSuffix)))
	}
	f := e.file
	e.file = nil
	return f.Close()
}
