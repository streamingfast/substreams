package bundler

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dhammer"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/sink/sql/db_changes/bundler/writer"
	"go.uber.org/zap"
)

type Bundler struct {
	*shutter.Shutter

	blockCount     uint64
	stats          *boundaryStats
	boundaryWriter writer.Writer
	outputStore    dstore.Store
	Header         []byte
	HeaderWritten  bool

	activeBoundary *bstream.Range
	stopBlock      uint64
	uploadQueue    *dhammer.Nailer
	zlogger        *zap.Logger
}

var ErrStopBlockReached = errors.New("stop block reached")

func New(
	size uint64,
	stopBlock uint64,
	boundaryWriter writer.Writer,
	outputStore dstore.Store,
	zlogger *zap.Logger,
	header []byte,
) (*Bundler, error) {

	b := &Bundler{
		Shutter:        shutter.New(),
		boundaryWriter: boundaryWriter,
		outputStore:    outputStore,
		blockCount:     size,
		stopBlock:      stopBlock,
		stats:          newStats(),
		zlogger:        zlogger,
		Header:         header,
		HeaderWritten:  false,
	}

	b.uploadQueue = dhammer.NewNailer(5, b.uploadBoundary, dhammer.NailerLogger(zlogger))

	return b, nil
}

func (b *Bundler) name() string {
	return path.Base(b.outputStore.BaseURL().Path)
}

func (b *Bundler) Launch(ctx context.Context) {
	b.OnTerminating(func(err error) {
		b.zlogger.Info("shutting down bundler", zap.String("store", b.name()), zap.Error(err))
		b.Close()
	})
	b.uploadQueue.Start(ctx)

	go func() {
		for v := range b.uploadQueue.Out {
			bf := v.(*boundaryFile)
			b.zlogger.Debug("uploaded file", zap.String("filename", bf.name))
		}
		if b.uploadQueue.Err() != nil {
			b.Shutdown(fmt.Errorf("upload queue failed: %w", b.uploadQueue.Err()))
		}
	}()

	b.uploadQueue.OnTerminating(func(_ error) {
		b.Shutdown(fmt.Errorf("upload queue failed: %w", b.uploadQueue.Err()))
	})
}

func (b *Bundler) Close() {
	b.zlogger.Debug("closing upload queue")
	b.uploadQueue.Close()
	b.zlogger.Debug("waiting till queue is drained")
	b.uploadQueue.WaitUntilEmpty(context.Background())
	b.zlogger.Debug("boundary upload completed")
}

func (b *Bundler) Roll(ctx context.Context, blockNum uint64) (rolled bool, err error) {
	if b.activeBoundary.Contains(blockNum) {
		return false, nil
	}

	boundaries := boundariesToSkip(b.activeBoundary, blockNum, b.blockCount)

	b.zlogger.Info("block_num is not in active boundary",
		zap.Stringer("active_boundary", b.activeBoundary),
		zap.Int("boundaries_to_skip", len(boundaries)),
		zap.Uint64("block_num", blockNum),
	)

	if err := b.stop(ctx); err != nil {
		return false, fmt.Errorf("stop active boundary: %w", err)
	}

	// Empty boundaries are before `blockNum`, we must flush them also before checking if we should quit
	for _, boundary := range boundaries {
		if err := b.Start(boundary.StartBlock()); err != nil {
			return false, fmt.Errorf("start skipping boundary: %w", err)
		}
		if err := b.stop(ctx); err != nil {
			return false, fmt.Errorf("stop skipping boundary: %w", err)
		}
	}

	if blockNum >= b.stopBlock {
		return false, ErrStopBlockReached
	}

	if err := b.Start(blockNum); err != nil {
		return false, fmt.Errorf("start active boundary: %w", err)
	}

	return true, nil
}

func (b *Bundler) TrackBlockProcessDuration(elapsed time.Duration) {
	b.stats.addProcessingDataDur(elapsed)
}

func (b *Bundler) Writer() writer.Writer {
	return b.boundaryWriter
}

func (b *Bundler) Start(blockNum uint64) error {
	boundaryRange := b.newBoundary(blockNum)
	b.activeBoundary = boundaryRange

	b.zlogger.Debug("starting new file boundary", zap.Stringer("boundary", boundaryRange))
	if err := b.boundaryWriter.StartBoundary(boundaryRange); err != nil {
		return fmt.Errorf("start file: %w", err)
	}

	b.stats.startBoundary(boundaryRange)
	b.zlogger.Debug("boundary started", zap.Stringer("boundary", boundaryRange))
	return nil
}

func (b *Bundler) stop(ctx context.Context) error {
	b.zlogger.Debug("stopping file boundary")

	file, err := b.boundaryWriter.CloseBoundary(ctx)
	if err != nil {
		return fmt.Errorf("closing file: %w", err)
	}

	if b.boundaryWriter.IsWritten() {
		b.zlogger.Debug("queuing boundary upload", zap.Stringer("boundary", b.activeBoundary))

		b.uploadQueue.In <- &boundaryFile{
			name: b.activeBoundary.String(),
			file: file,
		}
	} else {
		b.zlogger.Debug("boundary not written, skipping upload of files", zap.Stringer("boundary", b.activeBoundary))
	}

	// Reset state
	b.HeaderWritten = false
	b.activeBoundary = nil
	b.stats.endBoundary()

	b.zlogger.Info("bundler stats", b.stats.Log()...)
	return nil
}

func (b *Bundler) newBoundary(containingBlockNum uint64) *bstream.Range {
	startBlock := containingBlockNum - (containingBlockNum % b.blockCount)
	endBlock := startBlock + b.blockCount
	if b.stopBlock < endBlock {
		endBlock = b.stopBlock
	}
	return bstream.NewRangeExcludingEnd(startBlock, endBlock)
}

func boundariesToSkip(lastBoundary *bstream.Range, blockNum uint64, size uint64) (out []*bstream.Range) {
	iter := *lastBoundary.EndBlock()
	endBlock := computeEndBlock(iter, size)
	for blockNum >= endBlock {
		out = append(out, bstream.NewRangeExcludingEnd(iter, endBlock))
		iter = endBlock
		endBlock = computeEndBlock(iter, size)
	}
	return out
}

func computeEndBlock(startBlockNum, size uint64) uint64 {
	return (startBlockNum + size) - (startBlockNum+size)%size
}

type boundaryFile struct {
	name string
	file writer.Uploadeable
}

func (b *Bundler) uploadBoundary(ctx context.Context, v interface{}) (interface{}, error) {
	bf := v.(*boundaryFile)

	outputPath, err := bf.file.Upload(ctx, b.outputStore)
	if err != nil {
		return nil, fmt.Errorf("unable to upload: %w", err)
	}
	b.zlogger.Debug("boundary file uploaded",
		zap.String("boundary", bf.name),
		zap.String("output_path", outputPath),
	)

	return bf, nil
}
