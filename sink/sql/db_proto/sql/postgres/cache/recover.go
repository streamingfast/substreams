package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"go.uber.org/zap"
)

// recover decides the fate of everything already on disk, before a single new row is
// written.
//
// A segment is replayed only if it is sealed, intact, and not already recorded in the
// database. Anything else is removed: a torn write from a crash, a segment applied but
// not yet deleted, or anything sitting past a hole left by a lost segment. Whatever is
// discarded is re-streamed from the cursor, which costs Substreams throughput but can
// never corrupt.
func (c *Cache) recover(ctx context.Context) error {
	entries, err := os.ReadDir(c.options.Dir)
	if err != nil {
		return fmt.Errorf("listing %s: %w", c.options.Dir, err)
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "seg-") {
			dirs = append(dirs, entry.Name())
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	sort.Strings(dirs)

	applied, err := c.applier.AppliedSegments(ctx)
	if err != nil {
		return err
	}

	var (
		replayed  int
		discarded int
		holeFound bool
	)

	for _, name := range dirs {
		dir := filepath.Join(c.options.Dir, name)

		// Segment names carry a sequence number, so the highest one seen tells the next
		// run where to continue numbering.
		if sequence := parseSequence(name); sequence > c.nextSequence {
			c.nextSequence = sequence
		}

		if holeFound {
			os.RemoveAll(dir)
			discarded++
			continue
		}

		manifest, err := readManifest(dir)
		if err != nil || !manifest.Sealed {
			// Never sealed: the process died mid-segment. Its blocks are re-streamed.
			c.logger.Info("discarding an unsealed cache segment", zap.String("dir", dir))
			os.RemoveAll(dir)
			discarded++
			holeFound = true
			continue
		}

		if applied[manifest.FirstBlock] {
			// Committed, but the process died before the directory was removed.
			os.RemoveAll(dir)
			continue
		}

		if err := verify(dir, manifest); err != nil {
			c.logger.Warn("discarding a truncated cache segment, its blocks will be streamed again",
				zap.String("dir", dir), zap.Error(err))
			os.RemoveAll(dir)
			discarded++
			holeFound = true
			continue
		}

		if err := c.applier.Apply(ctx, dir, manifest); err != nil {
			return fmt.Errorf("replaying segment %s: %w", dir, err)
		}
		os.RemoveAll(dir)
		replayed++
	}

	if replayed > 0 || discarded > 0 {
		c.logger.Info("recovered the local cache",
			zap.Int("segments_replayed", replayed),
			zap.Int("segments_discarded", discarded))
	}

	return nil
}

// verify checks each data file against what the manifest recorded. Only the manifest is
// fsynced on seal, so a machine crash can leave a sealed segment short; the length check
// is what catches that before it reaches the database.
func verify(dir string, manifest *Manifest) error {
	for _, table := range manifest.Tables {
		path := filepath.Join(dir, table.File)

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s is missing: %w", table.File, err)
		}
		if info.Size() != table.Bytes {
			return fmt.Errorf("%s is %d bytes, the manifest recorded %d", table.File, info.Size(), table.Bytes)
		}

		if err := verifyTrailer(path); err != nil {
			return err
		}
	}

	return nil
}

// verifyTrailer checks the two bytes that terminate a binary COPY stream.
func verifyTrailer(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < int64(pgcopy.HeaderSize+pgcopy.TrailerSize) {
		return fmt.Errorf("%s is too short to be a pgcopy stream", filepath.Base(path))
	}

	trailer := make([]byte, pgcopy.TrailerSize)
	if _, err := file.ReadAt(trailer, info.Size()-int64(pgcopy.TrailerSize)); err != nil {
		return fmt.Errorf("reading the trailer of %s: %w", filepath.Base(path), err)
	}
	if trailer[0] != 0xFF || trailer[1] != 0xFF {
		return fmt.Errorf("%s does not end with a pgcopy trailer", filepath.Base(path))
	}

	return nil
}

func parseSequence(name string) uint64 {
	var sequence uint64
	if _, err := fmt.Sscanf(name, "seg-%d", &sequence); err != nil {
		return 0
	}

	return sequence
}
