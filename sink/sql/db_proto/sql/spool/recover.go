package spool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
func (b *Spool) recover(ctx context.Context) error {
	entries, err := os.ReadDir(b.options.Dir)
	if err != nil {
		return fmt.Errorf("listing %s: %w", b.options.Dir, err)
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
	slices.Sort(dirs)

	var (
		replayed  int
		discarded int
		holeFound bool
	)

	for _, name := range dirs {
		dir := filepath.Join(b.options.Dir, name)

		// Segment names carry a sequence number, so the highest one seen tells the next
		// run where to continue numbering.
		if sequence := parseSequence(name); sequence > b.nextSequence {
			b.nextSequence = sequence
		}

		if holeFound {
			os.RemoveAll(dir)
			discarded++
			continue
		}

		manifest, err := readManifest(dir)
		if err != nil || !manifest.Sealed {
			// Never sealed: the process died mid-segment. Its blocks are re-streamed.
			b.logger.Info("discarding an unsealed spool segment", zap.String("dir", dir))
			os.RemoveAll(dir)
			discarded++
			holeFound = true
			continue
		}

		already, err := b.applier.AlreadyApplied(ctx, manifest)
		if err != nil {
			return err
		}
		if already {
			// Already in the database, and the process died before the directory was
			// removed.
			os.RemoveAll(dir)
			continue
		}

		if err := b.codec.Verify(dir, manifest); err != nil {
			b.logger.Warn("discarding a truncated spool segment, its blocks will be streamed again",
				zap.String("dir", dir), zap.Error(err))
			os.RemoveAll(dir)
			discarded++
			holeFound = true
			continue
		}

		if err := b.applier.Apply(ctx, dir, manifest); err != nil {
			return fmt.Errorf("replaying segment %s: %w", dir, err)
		}
		os.RemoveAll(dir)
		replayed++
	}

	if replayed > 0 || discarded > 0 {
		b.logger.Info("recovered the local spool",
			zap.Int("segments_replayed", replayed),
			zap.Int("segments_discarded", discarded))
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
