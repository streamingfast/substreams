package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	// maxEmptyPartialSize is the size above which a partial file is never read to check
	// whether it is empty. An empty partial holds no bytes once decompressed, which zstd
	// stores in 13 bytes.
	maxEmptyPartialSize = 64

	emptyPartialProbeConcurrency = 32
	deletePartialConcurrency     = 16
)

// EmptyPartialKVs reports which of files, partial files of this store, are empty: no keys
// and no deleted prefixes. It lists the sizes of the files, then reads the first
// decompressed byte of the small ones only, so a large partial is never downloaded. A
// file that is missing, too large or unreadable is reported as not empty.
func (c *Config) EmptyPartialKVs(ctx context.Context, files []*FileInfo) (map[string]bool, error) {
	if len(files) == 0 {
		return nil, nil
	}

	sizes, err := c.partialFileSizes(ctx, files)
	if err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(files))
	var mu sync.Mutex
	var eg errgroup.Group
	eg.SetLimit(emptyPartialProbeConcurrency)
	for _, file := range files {
		size, found := sizes[file.Filename]
		if !found || size > maxEmptyPartialSize {
			continue
		}
		eg.Go(func() error {
			empty, err := c.isEmptyFile(ctx, file.Filename)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				zlog.Debug("cannot tell whether partial file is empty", zap.String("file_name", file.Filename), zap.Error(err))
				return nil
			}
			if empty {
				mu.Lock()
				out[file.Filename] = true
				mu.Unlock()
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// isEmptyFile reads at most one decompressed byte of filename.
func (c *Config) isEmptyFile(ctx context.Context, filename string) (bool, error) {
	reader, err := loadStoreStream(ctx, c.objStore, filename)
	if err != nil {
		return false, err
	}
	defer reader.Close()

	var b [1]byte
	_, err = io.ReadFull(reader, b[:])
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err
}

// partialFileSizes returns the size of each of files found in storage, keyed by file name.
// It lists the store rather than asking for each file: state files are named after their
// zero-padded end block, so the files of a range of segments sit next to each other in the
// listing.
func (c *Config) partialFileSizes(ctx context.Context, files []*FileInfo) (map[string]int64, error) {
	wanted := make(map[string]bool, len(files))
	var first, last string
	for _, file := range files {
		wanted[file.Filename] = true
		endBlock, _, _ := strings.Cut(file.Filename, "-")
		if first == "" || endBlock < first {
			first = endBlock
		}
		if endBlock > last {
			last = endBlock
		}
	}

	sizes := make(map[string]int64, len(files))
	for _, prefix := range listingPrefixes(first, last) {
		err := c.objStore.WalkAttributes(ctx, prefix, func(entry dstore.ObjectEntry) error {
			endBlock, _, _ := strings.Cut(entry.Name, "-")
			if len(endBlock) == len(last) && endBlock > last {
				return dstore.StopIteration
			}
			if name, ok := partialFileName(entry.Name); ok && wanted[name] {
				sizes[name] = entry.Size
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("listing partial files with prefix %q: %w", prefix, err)
		}
	}
	return sizes, nil
}

// listingPrefixes returns the name prefixes to list to see every file whose end block
// prefix lies between first and last, both the same length: their common prefix followed
// by each digit in between. It lists at most one decade of files outside the range.
func listingPrefixes(first, last string) []string {
	if len(first) != len(last) {
		return []string{""}
	}
	n := 0
	for n < len(first) && first[n] == last[n] {
		n++
	}
	if n == len(first) {
		return []string{first}
	}
	out := make([]string, 0, last[n]-first[n]+1)
	for d := first[n]; d <= last[n]; d++ {
		out = append(out, first[:n]+string(d))
	}
	return out
}

// partialFileName trims what a listing may add after a partial file name, like the store's
// extension, and reports whether name is a partial file at all.
func partialFileName(name string) (string, bool) {
	i := strings.Index(name, ".partial")
	if i < 0 {
		return "", false
	}
	return name[:i+len(".partial")], true
}

// CopyFullKV copies the full KV file ending at fromEnd to the one ending at toEnd, as is.
// Object stores that support it copy server-side, without the content leaving storage.
func (c *Config) CopyFullKV(ctx context.Context, fromEnd, toEnd uint64) error {
	from := NewCompleteFileInfo(c.name, c.moduleInitialBlock, fromEnd).Filename
	to := NewCompleteFileInfo(c.name, c.moduleInitialBlock, toEnd).Filename
	return derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		return c.objStore.CopyObject(ctx, from, to)
	})
}

// DeletePartialKVs deletes files, partial files of this store, and logs the ones it could
// not delete.
func (c *Config) DeletePartialKVs(ctx context.Context, files []*FileInfo) {
	var eg errgroup.Group
	eg.SetLimit(deletePartialConcurrency)
	for _, file := range files {
		eg.Go(func() error {
			if err := c.objStore.DeleteObject(ctx, file.Filename); err != nil {
				zlog.Warn("deleting partial file", zap.String("file_name", file.Filename), zap.Error(err))
			}
			return nil
		})
	}
	_ = eg.Wait()
}
