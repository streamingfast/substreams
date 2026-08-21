package spool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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

	// Said before the first segment is touched, not after the last.
	//
	// Recovery runs inside Open, which is before the sinker starts and before anything
	// else logs, and replaying a spool a killed backfill left behind is minutes of binary
	// COPY. Reporting only the summary at the end meant those minutes looked like a hang
	// at startup: the process was busy in the database, and the last thing it had said was
	// that it had read the sink info.
	b.logger.Info("recovering the local spool, the stream does not start until this finishes",
		zap.Int("segments_on_disk", len(dirs)),
		zap.String("dir", b.options.Dir))

	startedAt := time.Now()

	var (
		replayed  int
		discarded int
		holeFound bool
	)

	for index, name := range dirs {
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

		// Per segment rather than per batch of them: each one is a COPY of tens of
		// megabytes, so this is the only thing that says the wait is progressing and how
		// much of it is left.
		b.logger.Info("replaying a spool segment",
			zap.String("progress", fmt.Sprintf("%d/%d", index+1, len(dirs))),
			zap.Uint64("first_block", manifest.FirstBlock),
			zap.Uint64("last_block", manifest.LastBlock),
			zap.String("bytes", humanBytes(segmentBytes(manifest))))

		if err := b.applier.Apply(ctx, dir, manifest); err != nil {
			// Left to fail, because a segment the database refuses says something is wrong
			// with the rows or the schema, and dropping it quietly would hide that. It
			// does mean the sink cannot start until the cause is dealt with, so the way
			// out has to be said out loud rather than worked out from a stack trace.
			return fmt.Errorf("replaying segment %s: %w. The sink cannot start while it is there. "+
				"Deleting that directory drops the %d block(s) it holds, which are then streamed again from the stored cursor",
				dir, err, manifest.BlockCount())
		}
		os.RemoveAll(dir)
		replayed++
	}

	b.logger.Info("recovered the local spool",
		zap.Int("segments_replayed", replayed),
		zap.Int("segments_discarded", discarded),
		zap.Duration("elapsed", time.Since(startedAt)))

	return nil
}

func parseSequence(name string) uint64 {
	var sequence uint64
	if _, err := fmt.Sscanf(name, "seg-%d", &sequence); err != nil {
		return 0
	}

	return sequence
}
